//go:build !windows

package engine

import "os/exec"

// hideWindow 在非 Windows 平台无需隐藏控制台窗口。
func hideWindow(cmd *exec.Cmd) {}
