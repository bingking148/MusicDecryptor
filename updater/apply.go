package updater

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Progress 报告已下载字节和发布声明的总大小。
type Progress func(received, total int64)

// Install 下载比当前版本更新的 Windows 安装包，并安排在本进程退出后替换程序。
// 调用方在成功返回后退出进程。下载失败或没有合适的安装包时不会改动当前程序。
func Install(ctx context.Context, client *Client, current string, progress Progress) error {
	if client == nil {
		client = DefaultClient()
	}
	result, err := client.Check(ctx, current)
	if err != nil {
		return err
	}
	if result.AssetMissing {
		return fmt.Errorf("%s", result.Message)
	}
	if !result.Available {
		return fmt.Errorf("没有可安装的更新")
	}
	file, err := os.CreateTemp("", "MusicDecryptor-update-*.exe")
	if err != nil {
		return fmt.Errorf("下载失败：无法保存安装包")
	}
	path := file.Name()
	file.Close()
	if err := client.Download(ctx, result.AssetURL, result.AssetSize, path, progress); err != nil {
		os.Remove(path)
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		os.Remove(path)
		return fmt.Errorf("无法定位当前程序路径")
	}
	if err := startRelaunch(os.Getpid(), path, exe); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

// RelaunchScript 在当前进程退出后，用下载的安装包替换正在运行的程序并重新启动。
func RelaunchScript(pid int, src, dst string) string {
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$pidToWait = %d
$src = %s
$dst = %s
for ($i = 0; $i -lt 120; $i++) {
  if (-not (Get-Process -Id $pidToWait -ErrorAction SilentlyContinue)) { break }
  Start-Sleep -Milliseconds 500
}
if (Get-Process -Id $pidToWait -ErrorAction SilentlyContinue) { exit 1 }
Start-Sleep -Milliseconds 400
[System.IO.File]::Copy($src, $dst, $true)
Start-Process -FilePath $dst
Remove-Item -LiteralPath $src -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
`, pid, powershellQuote(src), powershellQuote(dst))
}

func powershellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
