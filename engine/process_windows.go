//go:build windows

package engine

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// hideWindow 让后台命令不在屏幕上弹黑框。
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// FindQQMusicPID 通过 tasklist 查找 QQMusic.exe 进程，返回第一个 PID。
func FindQQMusicPID() (uint32, error) {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq QQMusic.exe", "/FO", "CSV", "/NH")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(strings.ToLower(line), `"qqmusic.exe"`) {
			continue
		}
		parts := strings.Split(line, `","`)
		if len(parts) >= 2 {
			pid, err := strconv.ParseUint(strings.Trim(parts[1], `"`), 10, 32)
			if err == nil && pid > 0 {
				return uint32(pid), nil
			}
		}
	}
	return 0, fmt.Errorf("QQMusic.exe 未运行")
}

// FindProcessPath 用 PowerShell 查询进程的可执行文件路径。
func FindProcessPath(pid uint32) (string, error) {
	script := fmt.Sprintf("(Get-Process -Id %d -ErrorAction SilentlyContinue).Path", pid)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if path == "" || !strings.HasSuffix(strings.ToLower(path), ".exe") {
		return "", fmt.Errorf("无法获取进程路径")
	}
	return path, nil
}
