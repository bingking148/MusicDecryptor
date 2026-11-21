package engine

import (
	"fmt"
	"strings"
)

// NormalizeUserPath 把用户键入或粘贴的 Windows 路径整理成后续步骤可以直接使用的形式。
//
// 中文输入法在中文标点状态下会把盘符后的冒号打成全角「：」（U+FF1A），
// 全角模式下反斜杠会变成「＼」。这两种写法和半角几乎看不出差别，但路径解析只认半角。
// 资源管理器「复制为路径」以及中文引号还会在整段路径外面包一层引号。
// 这里先去掉这些外壳，再修正盘符位置的全角字符。
// 目录名里合法的全角冒号（如「歌曲：精选」）和全角斜杠「／」保持原样。
// 全角反斜杠「＼」会改成半角，否则 UNC 和目录分隔都会失效。
//
// 规则需与 frontend/dist/src/pathnorm.js 保持一致。
func NormalizeUserPath(path string) string {
	path = stripWrappingQuotes(path)
	path = strings.ReplaceAll(path, "＼", `\`)
	return normalizeDrivePrefix(path)
}

// normalizeLocalDir 规范化目录，并拒绝没有 Windows 卷名的路径。
// 返回的字符串会交给扫描和暂存目录创建，不能只用来改错误文案。
func normalizeLocalDir(dir string) (string, error) {
	dir = NormalizeUserPath(dir)
	if windowsVolumeName(dir) == "" {
		return "", fmt.Errorf("输出目录不是本地路径：%s", dir)
	}
	return dir, nil
}

func stripWrappingQuotes(path string) string {
	for {
		path = strings.TrimSpace(path)
		rs := []rune(path)
		if len(rs) < 2 || !isWrappingQuote(rs[0], rs[len(rs)-1]) {
			return path
		}
		path = string(rs[1 : len(rs)-1])
	}
}

func isWrappingQuote(open, close rune) bool {
	switch open {
	case '"':
		return close == '"'
	case '\'':
		return close == '\''
	case '“':
		return close == '”'
	case '‘':
		return close == '’'
	case '„':
		return close == '“' || close == '”'
	case '«':
		return close == '»'
	case '「':
		return close == '」'
	case '『':
		return close == '』'
	case '＂':
		return close == '＂'
	case '＇':
		return close == '＇'
	default:
		return false
	}
}

func normalizeDrivePrefix(path string) string {
	rs := []rune(path)
	if len(rs) < 2 {
		return path
	}
	letter, ok := foldFullwidthLetter(rs[0])
	if !ok || (rs[1] != ':' && rs[1] != '：') {
		return path
	}
	rs[0] = letter
	rs[1] = ':'
	return string(rs)
}

func foldFullwidthLetter(r rune) (rune, bool) {
	switch {
	case r >= 'A' && r <= 'Z':
		return r, true
	case r >= 'a' && r <= 'z':
		return r, true
	case r >= 'Ａ' && r <= 'Ｚ':
		return 'A' + (r - 'Ａ'), true
	case r >= 'ａ' && r <= 'ｚ':
		return 'a' + (r - 'ａ'), true
	default:
		return 0, false
	}
}

// windowsVolumeName 按 Go 标准库在 Windows 上的卷名规则识别本地路径。
// 不调用 filepath.VolumeName：非 Windows 环境它恒为空，也无法单独覆盖规范化之前的全角冒号。
// 逻辑对应 path/filepath.volumeNameLen（驱动器、UNC、\\?\ 与 \\.\ 设备路径）。
func windowsVolumeName(path string) string {
	n := windowsVolumeLen(path)
	if n <= 0 {
		return ""
	}
	return path[:n]
}

func windowsVolumeLen(path string) int {
	switch {
	case len(path) >= 2 && path[1] == ':':
		return 2
	case len(path) == 0 || !isPathSep(path[0]):
		return 0
	case pathHasPrefixFold(path, `\\.\UNC`):
		return uncVolumeLen(path, len(`\\.\UNC\`))
	case pathHasPrefixFold(path, `\\.`) || pathHasPrefixFold(path, `\\?`) || pathHasPrefixFold(path, `\??`):
		if len(path) == 3 {
			return 3
		}
		_, rest, ok := cutAtSep(path[4:])
		if !ok {
			return len(path)
		}
		return len(path) - len(rest) - 1
	case len(path) >= 2 && isPathSep(path[1]):
		return uncVolumeLen(path, 2)
	default:
		return 0
	}
}

func isPathSep(c byte) bool {
	return c == '\\' || c == '/'
}

func asciiUpper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

func pathHasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if isPathSep(prefix[i]) {
			if !isPathSep(s[i]) {
				return false
			}
		} else if asciiUpper(prefix[i]) != asciiUpper(s[i]) {
			return false
		}
	}
	if len(s) > len(prefix) && !isPathSep(s[len(prefix)]) {
		return false
	}
	return true
}

func uncVolumeLen(path string, prefixLen int) int {
	count := 0
	for i := prefixLen; i < len(path); i++ {
		if isPathSep(path[i]) {
			count++
			if count == 2 {
				return i
			}
		}
	}
	return len(path)
}

func cutAtSep(path string) (before, after string, found bool) {
	for i := 0; i < len(path); i++ {
		if isPathSep(path[i]) {
			return path[:i], path[i+1:], true
		}
	}
	return path, "", false
}
