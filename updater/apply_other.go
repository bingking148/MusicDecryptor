//go:build !windows

package updater

import "fmt"

func startRelaunch(pid int, src, dst string) error {
	return fmt.Errorf("应用内更新仅支持 Windows")
}
