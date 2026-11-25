package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func exeAsset(version string) Asset {
	return Asset{
		Name: "MusicDecryptor-v" + strings.TrimPrefix(version, "v") + "-windows-amd64.exe",
		URL:  "https://github.com/bingking148/MusicDecryptor/releases/download/" + version + "/app.exe",
		Size: 63_320_064,
	}
}

func TestSelectUpdate(t *testing.T) {
	current := "1.1.0"
	newer := Release{Tag: "v1.2.0", Body: "修复若干问题", PageURL: "https://github.com/bingking148/MusicDecryptor/releases/tag/v1.2.0", Assets: []Asset{exeAsset("v1.2.0")}}
	same := Release{Tag: "v1.1.0", Assets: []Asset{exeAsset("v1.1.0")}}
	older := Release{Tag: "v1.0.0", Assets: []Asset{exeAsset("v1.0.0")}}

	t.Run("更新版本提供安装", func(t *testing.T) {
		got, err := SelectUpdate(current, []Release{older, same, newer})
		if err != nil {
			t.Fatal(err)
		}
		if !got.Available || got.LatestVersion != "v1.2.0" || got.AssetName != newer.Assets[0].Name {
			t.Fatalf("结果不符合预期：%+v", got)
		}
		if got.AssetMissing {
			t.Fatal("安装包存在时不应标记缺失")
		}
	})

	t.Run("相同版本不提示", func(t *testing.T) {
		got, err := SelectUpdate(current, []Release{same, older})
		if err != nil {
			t.Fatal(err)
		}
		if got.Available || got.AssetMissing || got.Message != "已是最新版本" {
			t.Fatalf("结果不符合预期：%+v", got)
		}
	})

	t.Run("只有旧版本不提示", func(t *testing.T) {
		got, err := SelectUpdate(current, []Release{older})
		if err != nil {
			t.Fatal(err)
		}
		if got.Available || got.LatestVersion != "v1.0.0" {
			t.Fatalf("结果不符合预期：%+v", got)
		}
	})

	t.Run("忽略草稿和预发布", func(t *testing.T) {
		draft := newer
		draft.Tag = "v1.4.0"
		draft.Draft = true
		pre := newer
		pre.Tag = "v1.3.0"
		pre.Prerelease = true
		got, err := SelectUpdate(current, []Release{draft, pre, newer})
		if err != nil {
			t.Fatal(err)
		}
		if !got.Available || got.LatestVersion != "v1.2.0" {
			t.Fatalf("应选择正式版 v1.2.0，实际 %+v", got)
		}
	})

	t.Run("新版本没有 Windows 安装包", func(t *testing.T) {
		bad := newer
		bad.Assets = []Asset{{Name: "source.zip", URL: "https://github.com/bingking148/MusicDecryptor/archive.zip", Size: 10}}
		got, err := SelectUpdate(current, []Release{bad})
		if err != nil {
			t.Fatal(err)
		}
		if got.Available || !got.AssetMissing || !strings.Contains(got.Message, "没有可用的 Windows 安装包") {
			t.Fatalf("结果不符合预期：%+v", got)
		}
	})

	t.Run("非 amd64 安装包不采用", func(t *testing.T) {
		bad := newer
		bad.Assets = []Asset{{
			Name: "MusicDecryptor-v1.2.0-windows-386.exe",
			URL:  "https://github.com/bingking148/MusicDecryptor/releases/download/v1.2.0/x86.exe",
			Size: 100,
		}}
		got, err := SelectUpdate(current, []Release{bad})
		if err != nil {
			t.Fatal(err)
		}
		if got.Available || !got.AssetMissing {
			t.Fatalf("结果不符合预期：%+v", got)
		}
	})

	t.Run("多个候选安装包不猜测", func(t *testing.T) {
		bad := newer
		bad.Assets = []Asset{exeAsset("v1.2.0"), {
			Name: "MusicDecryptor-extra-windows-amd64.exe",
			URL:  "https://github.com/bingking148/MusicDecryptor/releases/download/v1.2.0/extra.exe",
			Size: 20,
		}}
		got, err := SelectUpdate(current, []Release{bad})
		if err != nil {
			t.Fatal(err)
		}
		if got.Available || !got.AssetMissing {
			t.Fatalf("结果不符合预期：%+v", got)
		}
	})

	t.Run("空下载地址视为不可用", func(t *testing.T) {
		bad := newer
		bad.Assets = []Asset{{Name: "MusicDecryptor-v1.2.0-windows-amd64.exe", URL: "", Size: 10}}
		got, err := SelectUpdate(current, []Release{bad})
		if err != nil {
			t.Fatal(err)
		}
		if got.Available || !got.AssetMissing {
			t.Fatalf("结果不符合预期：%+v", got)
		}
	})

	t.Run("空发布列表", func(t *testing.T) {
		got, err := SelectUpdate(current, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Available || got.Message != "没有已发布的版本" {
			t.Fatalf("结果不符合预期：%+v", got)
		}
	})
}

func TestCheckHTTP(t *testing.T) {
	newClient := func(body string, status int) *Client {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("User-Agent") == "" {
				t.Errorf("请求应带 User-Agent")
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(server.Close)
		return &Client{HTTP: server.Client(), ReleasesURL: server.URL, AllowLocal: true}
	}
	releaseJSON := `[
	  {"tag_name":"v1.0.0","draft":false,"prerelease":false,"assets":[{"name":"MusicDecryptor-v1.0.0-windows-amd64.exe","size":10,"browser_download_url":"https://github.com/bingking148/MusicDecryptor/releases/download/v1.0.0/a.exe"}]},
	  {"tag_name":"v1.2.0","draft":false,"prerelease":false,"html_url":"https://github.com/bingking148/MusicDecryptor/releases/tag/v1.2.0","body":"说明","assets":[{"name":"MusicDecryptor-v1.2.0-windows-amd64.exe","size":20,"browser_download_url":"https://github.com/bingking148/MusicDecryptor/releases/download/v1.2.0/b.exe"}]}
	]`

	t.Run("新版本", func(t *testing.T) {
		got, err := newClient(releaseJSON, http.StatusOK).Check(context.Background(), "1.1.0")
		if err != nil {
			t.Fatal(err)
		}
		if !got.Available || got.LatestVersion != "v1.2.0" || got.AssetSize != 20 {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("相同版本", func(t *testing.T) {
		got, err := newClient(releaseJSON, http.StatusOK).Check(context.Background(), "v1.2.0")
		if err != nil {
			t.Fatal(err)
		}
		if got.Available {
			t.Fatalf("相同版本不应提供更新：%+v", got)
		}
	})

	t.Run("空内容", func(t *testing.T) {
		_, err := newClient("   ", http.StatusOK).Check(context.Background(), "1.1.0")
		if err == nil || !strings.Contains(err.Error(), "空内容") {
			t.Fatalf("期望空内容错误，实际 %v", err)
		}
	})

	t.Run("无法识别", func(t *testing.T) {
		_, err := newClient(`{"message":"Not Found"}`, http.StatusOK).Check(context.Background(), "1.1.0")
		if err == nil || !strings.Contains(err.Error(), "无法识别") {
			t.Fatalf("期望无法识别，实际 %v", err)
		}
	})

	t.Run("null", func(t *testing.T) {
		_, err := newClient("null", http.StatusOK).Check(context.Background(), "1.1.0")
		if err == nil || !strings.Contains(err.Error(), "无法识别") {
			t.Fatalf("期望无法识别，实际 %v", err)
		}
	})

	t.Run("服务器错误", func(t *testing.T) {
		_, err := newClient("nope", http.StatusInternalServerError).Check(context.Background(), "1.1.0")
		if err == nil || !strings.Contains(err.Error(), "500") {
			t.Fatalf("期望 HTTP 错误，实际 %v", err)
		}
	})

	t.Run("空数组", func(t *testing.T) {
		got, err := newClient("[]", http.StatusOK).Check(context.Background(), "1.1.0")
		if err != nil {
			t.Fatal(err)
		}
		if got.Available || got.Message != "没有已发布的版本" {
			t.Fatalf("%+v", got)
		}
	})
}

func TestAllowedAssetURL(t *testing.T) {
	ok := []string{
		"https://github.com/bingking148/MusicDecryptor/releases/download/v1.2.0/MusicDecryptor-v1.2.0-windows-amd64.exe",
		"https://objects.githubusercontent.com/github-production-release-asset/app.exe",
		"https://release-assets.githubusercontent.com/github-production-release-asset/app.exe",
	}
	bad := []string{
		"",
		"http://github.com/a.exe",
		"https://example.com/app.exe",
		"https://github.com.evil.test/app.exe",
		"https://evilgithubusercontent.com/app.exe",
	}
	for _, raw := range ok {
		if !AllowedAssetURL(raw) {
			t.Errorf("应允许 %s", raw)
		}
	}
	for _, raw := range bad {
		if AllowedAssetURL(raw) {
			t.Errorf("应拒绝 %s", raw)
		}
	}
}
