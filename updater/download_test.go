package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestDownloadValidatesPayload(t *testing.T) {
	payload := []byte("MZ-music-decryptor-update")
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write(payload)
		case "/short":
			_, _ = w.Write(payload[:2])
		case "/bad":
			_, _ = w.Write([]byte("not-an-executable!!!!"))
		case "/empty":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &Client{HTTP: server.Client(), AllowLocal: true}
	dest := t.TempDir() + "/update.exe"

	t.Run("合法安装包", func(t *testing.T) {
		var last int64
		err := client.Download(context.Background(), server.URL+"/ok", int64(len(payload)), dest, func(received, total int64) {
			last = received
			if total != int64(len(payload)) {
				t.Fatalf("总大小 %d", total)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if last != int64(len(payload)) {
			t.Fatalf("进度停在 %d", last)
		}
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(payload) {
			t.Fatalf("文件内容不符")
		}
	})

	t.Run("大小不符", func(t *testing.T) {
		err := client.Download(context.Background(), server.URL+"/short", int64(len(payload)), dest, nil)
		if err == nil || !strings.Contains(err.Error(), "内容异常") && !strings.Contains(err.Error(), "大小") {
			t.Fatalf("期望大小或内容错误，实际 %v", err)
		}
	})

	t.Run("不是可执行文件", func(t *testing.T) {
		body := []byte("not-an-executable!!!!")
		err := client.Download(context.Background(), server.URL+"/bad", int64(len(body)), dest, nil)
		if err == nil || !strings.Contains(err.Error(), "内容异常") {
			t.Fatalf("期望内容异常，实际 %v", err)
		}
	})

	t.Run("空文件", func(t *testing.T) {
		err := client.Download(context.Background(), server.URL+"/empty", 4, dest, nil)
		if err == nil {
			t.Fatal("空文件应失败")
		}
	})

	t.Run("拒绝非 GitHub 地址", func(t *testing.T) {
		strict := &Client{}
		err := strict.Download(context.Background(), server.URL+"/ok", int64(len(payload)), dest, nil)
		if err == nil || !strings.Contains(err.Error(), "地址不可用") {
			t.Fatalf("期望拒绝本地测试地址，实际 %v", err)
		}
	})

	t.Run("异常大小直接拒绝", func(t *testing.T) {
		err := client.Download(context.Background(), server.URL+"/ok", 0, dest, nil)
		if err == nil || !strings.Contains(err.Error(), "大小异常") {
			t.Fatalf("期望大小异常，实际 %v", err)
		}
	})
}

func TestInstallStopsBeforeReplaceOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("安装会替换当前进程，不能在测试里执行")
	}
	payload := []byte("MZ-update")
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	body := `[{"tag_name":"v9.9.9","assets":[{"name":"MusicDecryptor-v9.9.9-windows-amd64.exe","size":9,"browser_download_url":"` + server.URL + `/app.exe"}]}]`
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/app.exe", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})

	client := &Client{HTTP: server.Client(), ReleasesURL: server.URL + "/releases", AllowLocal: true}
	err := Install(context.Background(), client, "1.1.0", nil)
	if err == nil || !strings.Contains(err.Error(), "Windows") {
		t.Fatalf("非 Windows 应在替换前停下，实际 %v", err)
	}
}
