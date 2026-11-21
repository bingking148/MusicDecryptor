//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func startRelaunch(pid int, src, dst string) error {
	file, err := os.CreateTemp("", "MusicDecryptor-relaunch-*.ps1")
	if err != nil {
		return fmt.Errorf("无法准备更新脚本")
	}
	path := file.Name()
	// PowerShell 5 默认按系统代码页读脚本，BOM 让中文路径保持原样。
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		file.Close()
		os.Remove(path)
		return fmt.Errorf("无法准备更新脚本")
	}
	if _, err := file.WriteString(RelaunchScript(pid, src, dst)); err != nil {
		file.Close()
		os.Remove(path)
		return fmt.Errorf("无法准备更新脚本")
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return fmt.Errorf("无法准备更新脚本")
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	if err := cmd.Start(); err != nil {
		os.Remove(path)
		return fmt.Errorf("无法启动更新程序")
	}
	return nil
}
