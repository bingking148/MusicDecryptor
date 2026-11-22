package engine

import (
	"bytes"
	"context"
	"crypto/md5"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------- 数据模型 ----------

type FileStatus string

const (
	StatusPending FileStatus = "pending"
	StatusRunning FileStatus = "running"
	StatusSuccess FileStatus = "success"
	StatusSkipped FileStatus = "skipped"
	StatusFailed  FileStatus = "failed"
)

type FileItem struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	RelPath string     `json:"relPath"`
	SrcPath string     `json:"srcPath"`
	OutPath string     `json:"outPath"`
	Size    int64      `json:"size"`
	Status  FileStatus `json:"status"`
	Message string     `json:"message,omitempty"`
}

type Summary struct {
	Total     int64 `json:"total"`
	Success   int64 `json:"success"`
	Skipped   int64 `json:"skipped"`
	Failed    int64 `json:"failed"`
	ElapsedMs int64 `json:"elapsedMs"`
	Cancelled bool  `json:"cancelled"`
	// StopReason 区分正常结束、用户取消、超时、注入器意外退出和脚本致命错误。
	// 取消与超时/注入器退出必须分开，界面据此显示不同的提示。
	StopReason string `json:"stopReason,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	// Unfinished 是本次结束时仍处于待处理或解密中、被收成失败的文件数。
	Unfinished int64 `json:"unfinished,omitempty"`
}

type Event = map[string]any

type Emitter func(kind string, data any)

// job 是发给注入脚本的单元：src 为 ASCII 暂存目录下的可视路径，tmp 为解密产物落盘位置。
type job struct {
	ID  string `json:"id"`
	Src string `json:"src"`
	Tmp string `json:"tmp"`
}

// ---------- 主流程 ----------

func Run(ctx context.Context, assets embed.FS, opts Options, emit Emitter, logf func(level, msg string)) (*Summary, error) {
	start := time.Now()
	summary := &Summary{}

	if st, err := os.Stat(opts.InputDir); err != nil || !st.IsDir() {
		return summary, fmt.Errorf("输入目录不存在或不可读：%s", opts.InputDir)
	}

	// 1. 扫描任务清单。已存在或重名的文件直接计入跳过，不送入注入器。
	items, err := scanTargets(opts)
	if err != nil {
		return summary, err
	}
	summary.Total = int64(len(items))
	pendingCount := 0
	for _, it := range items {
		if it.Status == StatusSkipped {
			summary.Skipped++
		} else {
			pendingCount++
		}
	}
	emit("files", items)
	emitProgress(emit, summary, start, false)
	logf("info", fmt.Sprintf("扫描到 %d 个文件：%d 个待处理，%d 个跳过", len(items), pendingCount, summary.Skipped))
	if len(items) == 0 {
		summary.Message = "没有找到 .mflac / .mgg 文件"
		summary.ElapsedMs = time.Since(start).Milliseconds()
		emitProgress(emit, summary, start, true)
		return summary, nil
	}

	// 2. 生成实际任务清单与 ASCII 暂存目录。
	staging, err := prepareStaging(opts.OutputDir)
	if err != nil {
		return summary, err
	}
	junction := filepath.Join(staging, "in")
	jobs := make([]job, 0, pendingCount)
	tmpByID := map[string]string{}
	for _, it := range items {
		if it.Status != StatusPending {
			continue
		}
		os.MkdirAll(filepath.Dir(it.OutPath), 0o755)
		tmp := filepath.Join(staging, fmt.Sprintf("work-%s.bin", it.ID))
		jobs = append(jobs, job{ID: it.ID, Src: filepath.Join(junction, it.RelPath), Tmp: tmp})
		tmpByID[it.ID] = tmp
	}
	if len(jobs) == 0 {
		summary.Message = "所有文件均已跳过"
		summary.ElapsedMs = time.Since(start).Milliseconds()
		emitProgress(emit, summary, start, true)
		return summary, nil
	}
	if err := ensureJunction(junction, opts.InputDir, logf); err != nil {
		return summary, err
	}
	jobsPath := filepath.Join(staging, "jobs.json")
	if err := writeJSONFile(jobsPath, jobs); err != nil {
		return summary, err
	}
	scriptPath := filepath.Join(staging, "hook.js")
	if err := writeScriptFile(assets, scriptPath, jobsPath); err != nil {
		return summary, err
	}

	// 4. 定位 QQ 音乐进程与注入器
	pid, err := FindQQMusicPID()
	if err != nil {
		return summary, fmt.Errorf("未检测到正在运行的 QQ 音乐客户端（QQMusic.exe），请先启动 QQ 音乐再试")
	}
	arch := "ia32"
	if path, err := FindProcessPath(pid); err == nil && path != "" {
		if machine, err := PEMachine(path); err == nil && machine == 0x8664 {
			arch = "x64"
		}
	}
	injector, err := ExtractInjector(assets, arch)
	if err != nil {
		return summary, err
	}
	logf("info", fmt.Sprintf("已定位 QQMusic.exe (pid=%d, %s)，注入器：%s", pid, arch, filepath.Base(injector)))

	// 5. 运行注入器并解析输出。进程退出和会话超时都会结束任务，不能只等脚本的 finished。
	cmd := exec.Command(injector, "-p", fmt.Sprint(pid), "-s", scriptPath)
	hideWindow(cmd)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return summary, fmt.Errorf("启动注入器失败：%v", err)
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	driveInjectorSession(ctx, stdout, stderr, waitDone, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}, items, summary, tmpByID, opts, emit, logf, start, injectIdleTimeout)

	cleanupStaging(staging, logf)
	return summary, nil
}

// Preview 只扫描并生成任务清单，不启动注入器。
func Preview(opts Options) ([]*FileItem, error) {
	NormalizeOptions(&opts)
	if st, err := os.Stat(opts.InputDir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("输入目录不存在或不可读：%s", opts.InputDir)
	}
	return scanTargets(opts)
}

// ---------- 扫描 ----------

func scanTargets(opts Options) ([]*FileItem, error) {
	var items []*FileItem
	seenOut := map[string]string{}
	included := map[string]bool{}
	for _, id := range opts.IncludeIDs {
		included[id] = true
	}

	root := opts.InputDir
	walk := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if !opts.Recursive && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".mflac" && ext != ".mgg" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		st, err := d.Info()
		if err != nil {
			return nil
		}

		outName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		outName += defaultOutExt(ext)
		outRel := outName
		if opts.KeepStructure {
			outRel = filepath.Join(filepath.Dir(rel), outName)
		}
		outPath := filepath.Join(opts.OutputDir, outRel)

		item := &FileItem{
			ID:      fmt.Sprintf("%x", md5.Sum([]byte(path)))[:12],
			Name:    filepath.Base(path),
			RelPath: rel,
			SrcPath: path,
			OutPath: outPath,
			Size:    st.Size(),
			Status:  StatusPending,
		}
		if len(included) > 0 && !included[item.ID] {
			return nil
		}

		// 同名输出冲突（不同子目录但未保留结构时会出现）
		if prev, ok := seenOut[strings.ToLower(outPath)]; ok {
			item.Status = StatusSkipped
			item.Message = "输出重名，已被「" + prev + "」占用"
		} else if opts.SkipExisting {
			if _, err := os.Stat(outPath); err == nil {
				item.Status = StatusSkipped
				item.Message = "输出已存在"
			}
		}
		if item.Status == StatusPending {
			seenOut[strings.ToLower(outPath)] = item.Name
		}
		items = append(items, item)
		return nil
	}

	if err := filepath.WalkDir(root, walk); err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].RelPath < items[j].RelPath })
	return items, nil
}

func defaultOutExt(srcExt string) string {
	if srcExt == ".mgg" {
		return ".ogg"
	}
	return ".flac"
}

// ---------- 暂存与 junction ----------

func prepareStaging(outputDir string) (string, error) {
	vol := filepath.VolumeName(outputDir)
	if vol == "" {
		return "", fmt.Errorf("输出目录不是本地路径：%s", outputDir)
	}
	staging := filepath.Join(vol+string(os.PathSeparator), "qqmusic_decrypt_tmp")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return "", fmt.Errorf("创建暂存目录失败：%v", err)
	}
	return staging, nil
}

// ensureJunction 建立「ASCII 暂存路径 → 输入目录」的目录联接，
// 绕开 QQ 音乐 DLL 无法打开非 ASCII 目录的限制。
func ensureJunction(junction, inputDir string, logf func(level, msg string)) error {
	// 暂存联接可能来自上一次任务，必须重建，避免继续指向旧的输入目录。
	if _, err := os.Lstat(junction); err == nil {
		if err := os.Remove(junction); err != nil {
			return fmt.Errorf("清理旧目录联接失败：%v", err)
		}
	}
	cmd := exec.Command("cmd", "/c", "mklink", "/J", junction, inputDir)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("创建目录联接失败（需要 NTFS 卷）：%v，输出：%s", err, strings.TrimSpace(string(out)))
	}
	logf("info", "已创建目录联接："+junction+" -> "+inputDir)
	return nil
}

func cleanupStaging(staging string, logf func(level, msg string)) {
	entries, err := os.ReadDir(staging)
	if err != nil {
		return
	}
	removed := 0
	for _, e := range entries {
		name := e.Name()
		if name == "in" { // 保留联接以便复用
			continue
		}
		if strings.HasSuffix(name, ".bin") || name == "jobs.json" || name == "hook.js" {
			os.Remove(filepath.Join(staging, name))
			removed++
		}
	}
	if removed > 0 {
		logf("info", fmt.Sprintf("清理暂存文件 %d 个", removed))
	}
}

// ---------- 消息解析与收尾 ----------

// parseScriptMessage 兼容多种 frida-inject 输出格式：
// {"type":"send","payload":{...}} / 直接的 payload 对象 / {"type":"error",...}
func parseScriptMessage(line string) map[string]any {
	var obj map[string]any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return nil
	}
	if t, ok := obj["t"]; ok {
		_ = t
		return obj // 已是 payload
	}
	switch obj["type"] {
	case "send":
		if payload, ok := obj["payload"].(map[string]any); ok {
			return payload
		}
	case "error":
		desc, _ := obj["description"].(string)
		if desc == "" {
			raw, _ := json.Marshal(obj)
			desc = string(raw)
		}
		return map[string]any{"t": "fatal", "err": desc}
	}
	return nil
}

func setItemStatus(items []*FileItem, id string, st FileStatus, msg string) {
	for _, it := range items {
		if it.ID == id {
			it.Status = st
			it.Message = msg
			return
		}
	}
}

// snapshotItem 在锁内复制一份文件状态，供锁外安全地序列化发送。
func snapshotItem(items []*FileItem, id string) *FileItem {
	for _, it := range items {
		if it.ID == id {
			cp := *it
			return &cp
		}
	}
	return nil
}

func emitProgress(emit Emitter, s *Summary, start time.Time, final bool) {
	done := s.Success + s.Skipped + s.Failed
	emit("progress", map[string]any{
		"total":     s.Total,
		"success":   s.Success,
		"skipped":   s.Skipped,
		"failed":    s.Failed,
		"done":      done,
		"elapsedMs": time.Since(start).Milliseconds(),
		"final":     final,
	})
}

// finalizeItem 把暂存产物改名到最终输出路径；AutoFixExt 时按真实格式修正扩展名。
func finalizeItem(items []*FileItem, id, tmpPath string, opts Options) string {
	var item *FileItem
	for _, it := range items {
		if it.ID == id {
			item = it
			break
		}
	}
	if item == nil {
		return "未知任务"
	}
	if _, err := os.Stat(tmpPath); err != nil {
		return "解密产物不存在"
	}

	outExt := defaultOutExt(strings.ToLower(filepath.Ext(item.SrcPath)))
	if opts.AutoFixExt {
		if sniff := sniffAudioExt(tmpPath); sniff != "" {
			outExt = sniff
		}
	}
	outPath := item.OutPath
	if ext := filepath.Ext(outPath); ext != "" && outExt != ext {
		outPath = strings.TrimSuffix(outPath, ext) + outExt
	}

	// Windows 的 Rename 不会覆盖已有文件；仅在用户明确关闭跳过时替换输出。
	if !opts.SkipExisting {
		if _, err := os.Stat(outPath); err == nil {
			if err := os.Remove(outPath); err != nil {
				return "覆盖已有输出失败：" + err.Error()
			}
		}
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		// 跨卷等场景退化为复制
		if err := copyFile(tmpPath, outPath); err != nil {
			return "写入输出失败：" + err.Error()
		}
		os.Remove(tmpPath)
	}
	item.OutPath = outPath
	return ""
}

// sniffAudioExt 依据文件头判断真实音频格式。
func sniffAudioExt(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var b [8]byte
	n, _ := io.ReadFull(f, b[:])
	if n <= 0 {
		return ""
	}
	switch {
	case bytes.HasPrefix(b[:n], []byte("fLaC")):
		return ".flac"
	case bytes.HasPrefix(b[:n], []byte("ID3")):
		return ".mp3"
	case bytes.HasPrefix(b[:n], []byte("OggS")):
		return ".ogg"
	case bytes.HasPrefix(b[:n], []byte("MThd")):
		return ".mid"
	case b[0] == 0xFF && (b[1]&0xE0) == 0xE0:
		return ".mp3"
	default:
		return ""
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func writeJSONFile(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// writeScriptFile 把注入脚本模板写入暂存目录，并填入任务清单路径。
func writeScriptFile(assets embed.FS, path, jobsPath string) error {
	tpl, err := assets.ReadFile("engine/assets/hook_qq_music.js")
	if err != nil {
		return fmt.Errorf("内置脚本缺失: %w", err)
	}
	script := strings.ReplaceAll(string(tpl), "__JOBS_PATH__", escapeJSString(jobsPath))
	return os.WriteFile(path, []byte(script), 0o644)
}

func escapeJSString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			if r < 32 {
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}
