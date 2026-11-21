package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testBridge(t *testing.T, decrypt FileDecryptor) (*PhoneBridge, BridgeInfo) {
	t.Helper()
	b := &PhoneBridge{
		Decrypt:  decrypt,
		TempRoot: t.TempDir(),
		MaxBytes: 1024,
	}
	info, err := b.start(0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Stop)
	return b, info
}

func pairToken(t *testing.T, info BridgeInfo) string {
	t.Helper()
	body := strings.NewReader(`{"code":"` + info.Code + `"}`)
	resp, err := http.Post("http://127.0.0.1:"+jsonNumber(info.Port)+"/v1/pair", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("pair status %d %s", resp.StatusCode, raw)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Token == "" {
		t.Fatal(err, out.Token)
	}
	return out.Token
}

func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestPhoneBridgeStaysClosedUntilStarted(t *testing.T) {
	b := &PhoneBridge{Decrypt: func(context.Context, string) (string, error) { return "", nil }}
	if b.Status().Enabled {
		t.Fatal("默认不应监听")
	}
	resp, err := http.Get("http://127.0.0.1:47321/v1/info")
	if err == nil {
		resp.Body.Close()
		t.Fatal("未开启时不应有服务")
	}
}

func TestPhoneBridgePairAndDecrypt(t *testing.T) {
	b, info := testBridge(t, func(ctx context.Context, src string) (string, error) {
		out := filepath.Join(filepath.Dir(filepath.Dir(src)), "out.flac")
		data, err := os.ReadFile(src)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(out, append([]byte("fLaC"), data...), 0o600); err != nil {
			return "", err
		}
		return out, nil
	})
	if info.Enabled != true || info.Code == "" {
		t.Fatalf("status %+v", info)
	}
	for _, addr := range info.Addrs {
		host := addr
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[:i]
		}
		if host == "127.0.0.1" || strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "172.") {
			continue
		}
		t.Fatalf("监听了非私网地址 %s", addr)
	}

	token := pairToken(t, info)
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+jsonNumber(info.Port)+"/v1/decrypt", strings.NewReader("cipher"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Filename", `..\..\secret.mflac`)
	req.ContentLength = int64(len("cipher"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("decrypt %d %s", resp.StatusCode, raw)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != "fLaCcipher" {
		t.Fatalf("body %q", got)
	}
	if resp.Header.Get("X-Audio-Ext") != ".flac" {
		t.Fatalf("ext %s", resp.Header.Get("X-Audio-Ext"))
	}
	_ = b
}

func TestPhoneBridgeRejectsBadAuthSizeAndPath(t *testing.T) {
	_, info := testBridge(t, func(context.Context, string) (string, error) {
		t.Fatal("不应进入解密")
		return "", nil
	})
	base := "http://127.0.0.1:" + jsonNumber(info.Port)

	resp, err := http.Post(base+"/v1/decrypt", "application/octet-stream", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无令牌应 401，得到 %d", resp.StatusCode)
	}

	for i := 0; i < phoneLockAfter; i++ {
		res, err := http.Post(base+"/v1/pair", "application/json", strings.NewReader(`{"code":"WRONG-CODE"}`))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if i < phoneLockAfter-1 && res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("第 %d 次应 401，得到 %d", i, res.StatusCode)
		}
	}
	locked, err := http.Post(base+"/v1/pair", "application/json", strings.NewReader(`{"code":"`+info.Code+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	locked.Body.Close()
	if locked.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("锁定后应 429，得到 %d", locked.StatusCode)
	}

	fresh, freshInfo := testBridge(t, func(context.Context, string) (string, error) {
		t.Fatal("错误扩展名不应解密")
		return "", nil
	})
	token := pairToken(t, freshInfo)
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+jsonNumber(freshInfo.Port)+"/v1/decrypt", strings.NewReader("abc"))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Filename", "notes.txt")
	req.ContentLength = 3
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("错误扩展名应 400，得到 %d", res.StatusCode)
	}

	big := strings.Repeat("a", 1100)
	req, _ = http.NewRequest(http.MethodPost, "http://127.0.0.1:"+jsonNumber(freshInfo.Port)+"/v1/decrypt", strings.NewReader(big))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Filename", "song.mgg")
	req.ContentLength = int64(len(big))
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("超限应 413，得到 %d", res.StatusCode)
	}
	_ = fresh
}

func TestPhoneBridgeCancelAndStop(t *testing.T) {
	started := make(chan struct{})
	_, info := testBridge(t, func(ctx context.Context, src string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	token := pairToken(t, info)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:"+jsonNumber(info.Port)+"/v1/decrypt", strings.NewReader("data"))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Filename", "a.mflac")
	req.ContentLength = 4
	done := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("解密没有开始")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("取消后请求没有结束")
	}
}

func TestSafeUploadName(t *testing.T) {
	name, err := safeUploadName(`..\..\Windows\secret.mflac`)
	if err != nil || name != "secret.mflac" {
		t.Fatalf("应只保留文件名，得到 %q %v", name, err)
	}
	if _, err := safeUploadName("song.mp3"); err == nil {
		t.Fatal("普通 mp3 应拒绝")
	}
}
