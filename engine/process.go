package engine

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

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
