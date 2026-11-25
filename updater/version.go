package updater

import (
	"fmt"
	"strconv"
	"strings"
)

// CurrentVersion 是应用版本的唯一来源，界面显示和 GitHub Release 比较都用它。
// 须与 wails.json 的 productVersion、build/windows/info.json 的 ProductVersion 一致。
// Windows 文件版本 file_version 用四段数字，1.1.2 对应 1.1.2.0。
const CurrentVersion = "1.1.2"

// Version 是去掉 v 前缀后的数字版本。Pre 非空表示预发布标记。
type Version struct {
	Major int
	Minor int
	Patch int
	Build int
	Pre   string
}

// ParseVersion 接受 v1.2.0、1.2.0、1.2.0.1 和带预发布后缀的写法。
func ParseVersion(raw string) (Version, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Version{}, fmt.Errorf("版本号为空")
	}
	if s[0] == 'v' || s[0] == 'V' {
		s = s[1:]
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	pre := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
		if pre == "" {
			return Version{}, fmt.Errorf("版本号无效：%s", raw)
		}
	}
	if s == "" {
		return Version{}, fmt.Errorf("版本号无效：%s", raw)
	}
	parts := strings.Split(s, ".")
	if len(parts) > 4 {
		return Version{}, fmt.Errorf("版本号无效：%s", raw)
	}
	nums := [4]int{}
	for i, part := range parts {
		if part == "" || !digitsOnly(part) {
			return Version{}, fmt.Errorf("版本号无效：%s", raw)
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, fmt.Errorf("版本号无效：%s", raw)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2], Build: nums[3], Pre: pre}, nil
}

// Compare 按数字版本比较。返回值大于 0 表示 v 更新。
// 数字相同时，正式版高于带预发布后缀的版本。
func (v Version) Compare(other Version) int {
	if c := cmpInt(v.Major, other.Major); c != 0 {
		return c
	}
	if c := cmpInt(v.Minor, other.Minor); c != 0 {
		return c
	}
	if c := cmpInt(v.Patch, other.Patch); c != 0 {
		return c
	}
	if c := cmpInt(v.Build, other.Build); c != 0 {
		return c
	}
	switch {
	case v.Pre == "" && other.Pre != "":
		return 1
	case v.Pre != "" && other.Pre == "":
		return -1
	case v.Pre == other.Pre:
		return 0
	case v.Pre < other.Pre:
		return -1
	default:
		return 1
	}
}

// Newer 报告 candidate 是否比 current 更新。两个字符串都必须是合法版本。
func Newer(current, candidate string) (bool, error) {
	cur, err := ParseVersion(current)
	if err != nil {
		return false, err
	}
	next, err := ParseVersion(candidate)
	if err != nil {
		return false, err
	}
	return next.Compare(cur) > 0, nil
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func digitsOnly(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
