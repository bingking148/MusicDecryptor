//go:build !windows

package engine

import "os/exec"

// hideWindow 在非 Windows 上没有控制台黑框需要隐藏。
func hideWindow(cmd *exec.Cmd) {}
