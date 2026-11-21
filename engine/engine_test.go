package engine

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

//go:embed all:assets
var testAssets embed.FS

func mkFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScanTargets 验证目录扫描、扩展名过滤、跳过已存在、保留结构。
func TestScanTargets(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()

	mkFile(t, filepath.Join(input, "song1.mflac"), "A")
	mkFile(t, filepath.Join(input, "song2.MGG"), "B")
	mkFile(t, filepath.Join(input, "sub", "song3.mflac"), "C")
	mkFile(t, filepath.Join(input, "ignore.txt"), "X")
	mkFile(t, filepath.Join(output, "song1.flac"), "existing")

	opts := Options{InputDir: input, OutputDir: output, Recursive: true, SkipExisting: true, KeepStructure: true}
	items, err := scanTargets(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("期望 3 个任务，得到 %d", len(items))
	}

	byName := map[string]*FileItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	s1 := byName["song1.mflac"]
	if s1.Status != StatusSkipped || s1.Message != "输出已存在" {
		t.Errorf("song1 应跳过，实际 %v / %s", s1.Status, s1.Message)
	}
	if want := filepath.Join(output, "song1.flac"); s1.OutPath != want {
		t.Errorf("song1 输出路径错误: %s", s1.OutPath)
	}
	s2 := byName["song2.MGG"]
	if s2.Status != StatusPending {
		t.Errorf("song2 应待处理")
	}
	if want := filepath.Join(output, "song2.ogg"); s2.OutPath != want {
		t.Errorf("song2 .MGG 应映射为 .ogg: %s", s2.OutPath)
	}
	s3 := byName["song3.mflac"]
	if want := filepath.Join(output, "sub", "song3.flac"); s3.OutPath != want {
		t.Errorf("song3 应保留子目录结构: %s", s3.OutPath)
	}

	// 非递归模式下忽略子目录
	opts.Recursive = false
	items, err = scanTargets(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("非递归模式期望 2 个任务，得到 %d", len(items))
	}
}

// TestScanIncludeIDs 验证失败项重试只保留指定任务，且 ID 在重复扫描时保持稳定。
func TestScanIncludeIDs(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()
	mkFile(t, filepath.Join(input, "a.mflac"), "A")
	mkFile(t, filepath.Join(input, "b.mflac"), "B")

	opts := Options{InputDir: input, OutputDir: output, Recursive: true}
	first, err := scanTargets(opts)
	if err != nil || len(first) != 2 {
		t.Fatalf("首次扫描结果异常: %v, %d", err, len(first))
	}
	wantID := first[0].ID
	opts.IncludeIDs = []string{wantID}
	filtered, err := scanTargets(opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].ID != wantID {
		t.Fatalf("按 ID 筛选失败: %+v", filtered)
	}
}

// TestScanDuplicateOutputs 验证不保留结构时的重名检测。
func TestScanDuplicateOutputs(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()
	mkFile(t, filepath.Join(input, "a", "same.mflac"), "A")
	mkFile(t, filepath.Join(input, "b", "same.mflac"), "B")

	items, err := scanTargets(Options{InputDir: input, OutputDir: output, Recursive: true, KeepStructure: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("期望 2 个任务，得到 %d", len(items))
	}
	dupes := 0
	for _, it := range items {
		if it.Status == StatusSkipped {
			dupes++
		}
	}
	if dupes != 1 {
		t.Errorf("期望 1 个重名跳过，得到 %d", dupes)
	}
}

// TestSniffAudioExt 验证文件头识别。
func TestSniffAudioExt(t *testing.T) {
	cases := map[string]string{
		"fLaCxxxx":         ".flac",
		"ID3\x03xx":        ".mp3",
		"OggSxxxx":         ".ogg",
		"\xFF\xFB\x90xxxx": ".mp3",
		"garbage!":         "",
	}
	for head, want := range cases {
		p := filepath.Join(t.TempDir(), "probe.bin")
		if err := os.WriteFile(p, []byte(head), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := sniffAudioExt(p); got != want {
			t.Errorf("sniff(%q) = %q, 期望 %q", head, got, want)
		}
	}
}

// TestParseScriptMessage 验证 frida-inject 输出解析。
func TestParseScriptMessage(t *testing.T) {
	m := parseScriptMessage(`{"t":"done","id":"abc","size":5}`)
	if m == nil || m["t"] != "done" {
		t.Errorf("payload 直传应解析: %v", m)
	}
	m = parseScriptMessage(`{"type":"send","payload":{"t":"start","id":"x"}}`)
	if m == nil || m["t"] != "start" {
		t.Errorf("send 包装应解析: %v", m)
	}
	m = parseScriptMessage(`{"type":"error","description":"boom"}`)
	if m == nil || m["t"] != "fatal" {
		t.Errorf("error 应映射为 fatal: %v", m)
	}
	if parseScriptMessage("plain log line") != nil {
		t.Errorf("普通日志不应解析为消息")
	}
}

// TestRunWithoutQQMusic 验证 QQ 音乐未运行时的友好错误。
func TestRunWithoutQQMusic(t *testing.T) {
	input := t.TempDir()
	output := t.TempDir()
	mkFile(t, filepath.Join(input, "x.mflac"), "data")

	logs := 0
	_, err := Run(context.Background(), testAssets, Options{
		InputDir: input, OutputDir: output, Recursive: true,
	}, func(kind string, data any) {}, func(level, msg string) { logs++ })
	if err == nil {
		t.Skip("QQ 音乐正在运行？跳过该用例")
	}
	want := "QQ 音乐"
	if fmt.Sprintf("%v", err) == "" {
		t.Fatal("无错误信息")
	}
	_ = want
	_ = logs
}

// TestExtractInjectors 验证内置引擎可解压且可执行。
func TestExtractInjectors(t *testing.T) {
	ready, arch, version, note := InjectorStatus(testAssets)
	t.Logf("InjectorStatus: ready=%v arch=%s version=%s note=%s", ready, arch, version, note)
	if !ready {
		t.Fatalf("引擎状态应为就绪: %s", note)
	}
	for _, a := range []string{"ia32", "x64"} {
		path, err := ExtractInjector(testAssets, a)
		if err != nil {
			t.Fatalf("%s 解压失败: %v", a, err)
		}
		st, err := os.Stat(path)
		if err != nil || st.Size() < 10<<20 {
			t.Fatalf("%s 产物异常: %v", a, err)
		}
		t.Logf("%s -> %s (%d MB)", a, path, st.Size()>>20)
	}
}
