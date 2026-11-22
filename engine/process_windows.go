//go:build windows

package engine

import (
	"os/exec"
	"syscall"
)

// hideWindow 让后台命令不在屏幕上弹黑框。
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
