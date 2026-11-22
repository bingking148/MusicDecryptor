package updater

import (
	"runtime"
	"strings"
	"testing"
)

func TestRelaunchScriptQuotesPaths(t *testing.T) {
	script := RelaunchScript(4321, `C:\Temp\更新 '新'.exe`, `D:\音乐工具\MusicDecryptor.exe`)
	if !strings.Contains(script, "$pidToWait = 4321") {
		t.Fatalf("脚本应等待当前进程：%s", script)
	}
	if !strings.Contains(script, `'C:\Temp\更新 ''新''.exe'`) {
		t.Fatalf("来源路径引号不正确：%s", script)
	}
	if !strings.Contains(script, `'D:\音乐工具\MusicDecryptor.exe'`) {
		t.Fatalf("目标路径不正确：%s", script)
	}
	if !strings.Contains(script, "[System.IO.File]::Copy($src, $dst, $true)") {
		t.Fatalf("脚本应覆盖当前程序")
	}
	if !strings.Contains(script, "Start-Process -FilePath $dst") {
		t.Fatalf("脚本应重新启动程序")
	}
	if strings.Contains(script, "$pid =") {
		t.Fatalf("不应占用 PowerShell 的 $pid 变量")
	}
}

func TestStartRelaunchOtherPlatform(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 会真正启动替换脚本")
	}
	err := startRelaunch(1, "new.exe", "app.exe")
	if err == nil || !strings.Contains(err.Error(), "Windows") {
		t.Fatalf("期望仅支持 Windows，实际 %v", err)
	}
}
