package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVersionMetadataMatches(t *testing.T) {
	root := moduleRoot(t)
	wailsRaw, err := os.ReadFile(filepath.Join(root, "wails.json"))
	if err != nil {
		t.Fatal(err)
	}
	var wails struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(wailsRaw, &wails); err != nil {
		t.Fatal(err)
	}
	if wails.Info.ProductVersion != CurrentVersion {
		t.Fatalf("wails.json productVersion = %q，应用版本 = %q", wails.Info.ProductVersion, CurrentVersion)
	}

	infoRaw, err := os.ReadFile(filepath.Join(root, "build", "windows", "info.json"))
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Fixed struct {
			FileVersion string `json:"file_version"`
		} `json:"fixed"`
		Info map[string]struct {
			ProductVersion string `json:"ProductVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(infoRaw, &info); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseVersion(CurrentVersion)
	if err != nil {
		t.Fatal(err)
	}
	wantFile := fmt.Sprintf("%d.%d.%d.%d", parsed.Major, parsed.Minor, parsed.Patch, parsed.Build)
	if info.Fixed.FileVersion != wantFile {
		t.Fatalf("file_version = %q，期望 %q", info.Fixed.FileVersion, wantFile)
	}
	product := info.Info["0409"].ProductVersion
	if product != CurrentVersion {
		t.Fatalf("ProductVersion = %q，期望 %q", product, CurrentVersion)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位测试文件")
	}
	return filepath.Dir(filepath.Dir(file))
}
