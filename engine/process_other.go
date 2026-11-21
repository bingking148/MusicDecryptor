//go:build !windows

package engine

import (
	"fmt"
	"os/exec"
)

func hideWindow(cmd *exec.Cmd) {}

func FindQQMusicPID() (uint32, error) {
	return 0, fmt.Errorf("当前系统没有 QQ 音乐桌面客户端")
}

func FindProcessPath(pid uint32) (string, error) {
	return "", fmt.Errorf("当前系统无法查询 QQ 音乐进程")
}
