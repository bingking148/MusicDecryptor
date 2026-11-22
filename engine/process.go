package engine

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

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

// PEMachine 读取 PE 头的 Machine 字段（0x14C=x86, 0x8664=x64）。
func PEMachine(path string) (uint16, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var header [0x40]byte
	if _, err := io.ReadFull(f, header[:2]); err != nil {
		return 0, err
	}
	if header[0] != 'M' || header[1] != 'Z' {
		return 0, fmt.Errorf("不是 PE 文件")
	}
	var peOff [4]byte
	if _, err := f.ReadAt(peOff[:], 0x3C); err != nil {
		return 0, err
	}
	off := binary.LittleEndian.Uint32(peOff[:])
	var peHeader [6]byte
	if _, err := f.ReadAt(peHeader[:], int64(off)); err != nil {
		return 0, err
	}
	if peHeader[0] != 'P' || peHeader[1] != 'E' {
		return 0, fmt.Errorf("PE 签名无效")
	}
	machine := binary.LittleEndian.Uint16(peHeader[4:])
	return machine, nil
}

func PEArchName(path string) string {
	machine, err := PEMachine(path)
	if err != nil {
		return "unknown"
	}
	switch machine {
	case 0x14C:
		return "x86 (32位)"
	case 0x8664:
		return "x64 (64位)"
	default:
		return fmt.Sprintf("0x%X", machine)
	}
}

// ensureASCII 检查字符串是否全部为 ASCII（QQ 音乐 DLL 对非 ASCII 目录的兼容性要求）。
func ensureASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}
