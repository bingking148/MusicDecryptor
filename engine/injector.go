package engine

import (
	"embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ulikunitz/xz"
)

// frida-inject 官方二进制（frida 17.27.4），按 QQ 音乐客户端位数选择。
// 解密本身由 QQ 音乐自带 DLL 完成，frida 只负责把脚本注入到客户端进程。
const (
	injectorIA32 = "frida-inject-ia32.exe"
	injectorX64  = "frida-inject-x64.exe"
)

var (
	assetIA32 = "engine/assets/frida-inject-ia32.exe.xz"
	assetX64  = "engine/assets/frida-inject-x64.exe.xz"
)

// readAsset 兼容两种 embed 前缀：主程序从模块根嵌入（engine/assets/...），
// 测试从包目录嵌入（assets/...）。
func readAsset(assets embed.FS, name string) ([]byte, error) {
	data, err := assets.ReadFile(name)
	if err == nil {
		return data, nil
	}
	if alt, err2 := assets.ReadFile(strings.TrimPrefix(name, "engine/")); err2 == nil {
		return alt, nil
	}
	return nil, err
}

// injectorDir 返回注入器的存放目录：优先应用目录（便携），否则 LOCALAPPDATA。
func injectorDir() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "bin")
		if dirIsWritable(filepath.Dir(exe)) {
			return dir
		}
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "MusicDecryptorDesktop", "bin")
}

func dirIsWritable(dir string) bool {
	test := filepath.Join(dir, fmt.Sprintf(".write-test-%d", time.Now().UnixNano()))
	f, err := os.Create(test)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(test)
	return true
}

// extractMu 串行化解压流程：环境轮询会并发调用本函数，
// Windows 下对正在写入的 .part 文件 rename 会失败，必须互斥。
var extractMu sync.Mutex

// ExtractInjector 确保指定架构的注入器已解压到磁盘，返回其路径。
// 用户如果手动在 bin 目录放置了同名文件，则直接使用用户提供的版本。
func ExtractInjector(assets embed.FS, arch string) (string, error) {
	extractMu.Lock()
	defer extractMu.Unlock()

	name := injectorIA32
	asset := assetIA32
	if arch == "x64" {
		name = injectorX64
		asset = assetX64
	}

	dir := injectorDir()
	target := filepath.Join(dir, name)

	if st, err := os.Stat(target); err == nil && st.Size() > 1<<20 {
		return target, nil // 用户自备或已解压
	}

	data, err := readAsset(assets, asset)
	if err != nil {
		return "", fmt.Errorf("内置注入器资源缺失: %w", err)
	}
	xr, err := xz.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("解压器初始化失败: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp := target + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, xr); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return target, nil
}

// EnsureInjectors 解压两种架构的注入器（用于提前预热）。
func EnsureInjectors(assets embed.FS) error {
	var firstErr error
	for _, arch := range []string{"ia32", "x64"} {
		if _, err := ExtractInjector(assets, arch); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// InjectorStatus 检查注入器可用性，供前端环境面板展示。
func InjectorStatus(assets embed.FS) (ready bool, arch, version, note string) {
	for _, a := range []string{"ia32", "x64"} {
		if path, err := ExtractInjector(assets, a); err == nil {
			v := queryInjectorVersion(path)
			if v != "" {
				return true, a, v, "引擎就绪（frida-inject " + v + "）"
			}
			return true, a, "?", "引擎已就绪"
		}
	}
	return false, "", "", "引擎未就绪：无法解压内置注入器"
}

func queryInjectorVersion(path string) string {
	cmd := exec.Command(path, "--version")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
