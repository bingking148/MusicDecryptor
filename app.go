package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"musicdecryptor/engine"
	"musicdecryptor/updater"
)

// App 桌面应用绑定层：把引擎能力暴露给前端。
type App struct {
	ctx    context.Context
	assets embed.FS

	mu          sync.Mutex
	running     bool
	cancel      context.CancelFunc
	logLines    []string
	settings    *engine.Settings
	qqPathCache *qqPathCache
}

type qqPathCache struct {
	pid  uint32
	path string
	arch string
	at   time.Time
}

func NewApp(engineAssets embed.FS) *App {
	return &App{assets: engineAssets}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.settings = engine.LoadSettings()

	// 后台准备引擎（解压 frida-inject），完成后广播环境状态
	go func() {
		_ = engine.EnsureInjectors(a.assets)
		a.emitEnvironment()
	}()
}

func (a *App) shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) emitEnvironment() {
	runtime.EventsEmit(a.ctx, "engine:environment", a.CheckEnvironment())
}

// ---------- 窗口控制（无边框窗口的自定义标题栏） ----------

func (a *App) WindowMinimise() { runtime.WindowMinimise(a.ctx) }

func (a *App) WindowToggleMaximise() { runtime.WindowToggleMaximise(a.ctx) }

func (a *App) WindowClose() { runtime.Quit(a.ctx) }

// ---------- 对话框 ----------

func (a *App) SelectFolder(title string) string {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                title,
		CanCreateDirectories: true,
	})
	if err != nil {
		return ""
	}
	return path
}

func (a *App) OpenPath(path string) {
	if path == "" {
		return
	}
	exec.Command("explorer", "/select,", path).Start()
}

func (a *App) OpenDirectory(path string) {
	if path == "" {
		return
	}
	exec.Command("explorer", path).Start()
}

// ResolveDropPath 把拖入的文件或目录统一转换为待扫描目录。
func (a *App) ResolveDropPath(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return path
	}
	return filepath.Dir(path)
}

// ---------- 设置 ----------

func (a *App) GetSettings() *engine.Settings {
	return a.settings
}

func (a *App) SaveSettings(s *engine.Settings) *engine.Settings {
	a.settings = engine.NormalizeSettings(s)
	_ = engine.SaveSettings(a.settings)
	return a.settings
}

// ScanFiles 预扫描任务，供前端在解密前检查文件与输出路径。
func (a *App) ScanFiles(opts engine.Options) ([]*engine.FileItem, error) {
	engine.NormalizeOptions(&opts)
	return engine.Preview(opts)
}

// ---------- 环境检测 ----------

type Environment struct {
	AppVersion     string `json:"appVersion"`
	QQMusicRunning bool   `json:"qqMusicRunning"`
	QQMusicPath    string `json:"qqMusicPath"`
	QQMusicArch    string `json:"qqMusicArch"`
	EngineReady    bool   `json:"engineReady"`
	EngineArch     string `json:"engineArch"`
	EngineVersion  string `json:"engineVersion"`
	Note           string `json:"note"`
}

func (a *App) CheckEnvironment() Environment {
	env := Environment{AppVersion: updater.CurrentVersion}

	if pid, err := engine.FindQQMusicPID(); err == nil && pid != 0 {
		env.QQMusicRunning = true
		a.mu.Lock()
		cached := a.qqPathCache
		a.mu.Unlock()
		if cached != nil && cached.pid == pid && time.Since(cached.at) < 60*time.Second {
			env.QQMusicPath = cached.path
			env.QQMusicArch = cached.arch
		} else if path, err := engine.FindProcessPath(pid); err == nil && path != "" {
			env.QQMusicPath = path
			env.QQMusicArch = engine.PEArchName(path)
			a.mu.Lock()
			a.qqPathCache = &qqPathCache{pid: pid, path: path, arch: env.QQMusicArch, at: time.Now()}
			a.mu.Unlock()
		}
	}

	ready, arch, version, note := engine.InjectorStatus(a.assets)
	env.EngineReady = ready
	env.EngineArch = arch
	env.EngineVersion = version
	env.Note = note
	return env
}

// CheckForUpdate 在后台查询 GitHub Releases。没有更新时 Available 为 false，不打断当前任务。
func (a *App) CheckForUpdate() (*updater.Result, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	result, err := updater.DefaultClient().Check(ctx, updater.CurrentVersion)
	if err != nil {
		a.appendLog("warn", err.Error())
		return nil, err
	}
	if result.Available {
		a.appendLog("info", "发现新版本 "+result.LatestVersion)
	} else if result.AssetMissing {
		a.appendLog("warn", result.Message)
	}
	return &result, nil
}

// InstallUpdate 在用户确认后下载安装包，并在当前程序退出后完成替换和重启。
func (a *App) InstallUpdate() error {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	if running {
		return fmt.Errorf("当前任务仍在进行，请先停止后再更新")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	err := updater.Install(ctx, updater.DefaultClient(), updater.CurrentVersion, func(received, total int64) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "update:progress", map[string]any{
				"received": received,
				"total":    total,
			})
		}
	})
	if err != nil {
		a.appendLog("error", err.Error())
		return err
	}
	a.appendLog("info", "更新包已就绪，即将重启")
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
	return nil
}

// ---------- 解密 ----------

func (a *App) appendLog(level, line string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logLines = append(a.logLines, fmt.Sprintf("%s [%s] %s", time.Now().Format("15:04:05"), level, line))
	if len(a.logLines) > 4000 {
		a.logLines = a.logLines[len(a.logLines)-4000:]
	}
}

// StartDecrypt 启动一次批量解密任务，结果通过 Wails 事件推送给前端：
//   - engine:files    {items: []engine.FileItem}      任务清单
//   - engine:file     {item: engine.FileItem}         单个文件状态更新
//   - engine:progress {total,success,skipped,failed,done,elapsedMs}
//   - engine:summary  {total,success,skipped,failed,elapsedMs,cancelled}
//   - engine:log      {level,message}
func (a *App) StartDecrypt(opts engine.Options) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("已有任务正在进行中")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.running = true
	a.cancel = cancel
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.running = false
			a.cancel = nil
			a.mu.Unlock()
			cancel()
		}()

		emit := func(kind string, payload any) {
			runtime.EventsEmit(a.ctx, "engine:event", map[string]any{"kind": kind, "data": payload})
		}
		logf := func(level, msg string) {
			a.appendLog(level, msg)
			emit("log", map[string]string{"level": level, "message": msg})
		}

		engine.NormalizeOptions(&opts)
		logf("info", fmt.Sprintf("任务开始：输入=%s 输出=%s", opts.InputDir, opts.OutputDir))

		summary, err := engine.Run(ctx, a.assets, opts, emit, logf)
		if err != nil {
			if ctx.Err() == context.Canceled {
				logf("warn", "任务已取消")
				emit("summary", map[string]any{"cancelled": true, "elapsedMs": 0})
			} else {
				logf("error", err.Error())
				emit("error", map[string]string{"message": err.Error()})
			}
			return
		}
		logf("info", fmt.Sprintf("任务完成：成功 %d，跳过 %d，失败 %d，耗时 %.1fs",
			summary.Success, summary.Skipped, summary.Failed, float64(summary.ElapsedMs)/1000))
		emit("summary", summary)
	}()

	return nil
}

func (a *App) CancelDecrypt() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

// GetLog 返回当前会话的完整日志。
func (a *App) GetLog() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Join(a.logLines, "\n")
}

// ClearLog 同时清空后端日志，确保导出内容与界面一致。
func (a *App) ClearLog() {
	a.mu.Lock()
	a.logLines = nil
	a.mu.Unlock()
}

// ExportLog 把日志保存到用户选择的位置。
func (a *App) ExportLog() string {
	a.mu.Lock()
	content := strings.Join(a.logLines, "\n")
	a.mu.Unlock()

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出日志",
		DefaultFilename: fmt.Sprintf("decrypt-log-%s.txt", time.Now().Format("20060102-150405")),
	})
	if err != nil || path == "" {
		return ""
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return ""
	}
	return path
}

// CopyText 复制文本到剪贴板（日志复制按钮用）。
func (a *App) CopyText(text string) bool {
	runtime.ClipboardSetText(a.ctx, text)
	return true
}
