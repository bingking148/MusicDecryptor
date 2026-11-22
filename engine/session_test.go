package engine

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type emitted struct {
	kind string
	data any
}

type sessionHarness struct {
	stdoutW *io.PipeWriter
	stderrW *io.PipeWriter
	exited  chan error
	kills   atomic.Int32

	mu     sync.Mutex
	events []emitted
	logs   []string

	stdoutR *io.PipeReader
	stderrR *io.PipeReader
}

func newSessionHarness(t *testing.T) *sessionHarness {
	t.Helper()
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	h := &sessionHarness{
		stdoutR: stdoutR,
		stdoutW: stdoutW,
		stderrR: stderrR,
		stderrW: stderrW,
		exited:  make(chan error, 1),
	}
	t.Cleanup(func() {
		_ = stdoutW.Close()
		_ = stderrW.Close()
		_ = stdoutR.Close()
		_ = stderrR.Close()
	})
	return h
}

func (h *sessionHarness) kill() {
	h.kills.Add(1)
	_ = h.stdoutW.Close()
	_ = h.stderrW.Close()
	select {
	case h.exited <- fmt.Errorf("killed"):
	default:
	}
}

func (h *sessionHarness) emit(kind string, data any) {
	h.mu.Lock()
	h.events = append(h.events, emitted{kind: kind, data: data})
	h.mu.Unlock()
}

func (h *sessionHarness) logf(level, msg string) {
	h.mu.Lock()
	h.logs = append(h.logs, level+" "+msg)
	h.mu.Unlock()
}

func (h *sessionHarness) drive(ctx context.Context, items []*FileItem, summary *Summary, idle time.Duration) string {
	return driveInjectorSession(ctx, h.stdoutR, h.stderrR, h.exited, h.kill, items, summary, map[string]string{}, Options{}, h.emit, h.logf, time.Now(), idle)
}

func (h *sessionHarness) mustDrive(t *testing.T, ctx context.Context, items []*FileItem, summary *Summary, idle time.Duration) string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		done <- h.drive(ctx, items, summary, idle)
	}()
	select {
	case reason := <-done:
		return reason
	case <-time.After(3 * time.Second):
		t.Fatal("注入会话没有结束")
		return ""
	}
}

func (h *sessionHarness) notices(t *testing.T) []map[string]any {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []map[string]any
	for _, ev := range h.events {
		if ev.kind != "notice" {
			continue
		}
		payload, ok := ev.data.(map[string]any)
		if !ok {
			t.Fatalf("notice 数据类型错误: %T", ev.data)
		}
		out = append(out, payload)
	}
	return out
}

func (h *sessionHarness) finalProgress(t *testing.T) map[string]any {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	var last map[string]any
	for _, ev := range h.events {
		if ev.kind != "progress" {
			continue
		}
		payload, ok := ev.data.(map[string]any)
		if !ok {
			t.Fatalf("progress 数据类型错误: %T", ev.data)
		}
		last = payload
	}
	if last == nil || last["final"] != true {
		t.Fatalf("缺少最终进度事件: %#v", last)
	}
	return last
}

func (h *sessionHarness) logText() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.logs, "\n")
}

func countFileEvents(t *testing.T, h *sessionHarness, status FileStatus, message string) int {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, ev := range h.events {
		if ev.kind != "file" {
			continue
		}
		item, ok := ev.data.(*FileItem)
		if !ok {
			t.Fatalf("file 事件数据类型错误: %T", ev.data)
		}
		if item.Status == status && item.Message == message {
			n++
		}
	}
	return n
}

func countStatus(items []*FileItem, status FileStatus, message string) int {
	n := 0
	for _, it := range items {
		if it.Status == status && (message == "" || it.Message == message) {
			n++
		}
	}
	return n
}

// TestInjectorTimeoutCompletesJob 复现注入器超时退出：没有 finished，
// stderr 为 "Timeout was reached"。未完成文件必须进入失败终态，并发出主界面通知。
func TestInjectorTimeoutCompletesJob(t *testing.T) {
	h := newSessionHarness(t)
	items := []*FileItem{
		{ID: "a", Name: "a.mflac", Status: StatusRunning},
		{ID: "b", Name: "b.mflac", Status: StatusPending},
		{ID: "c", Name: "c.mflac", Status: StatusPending},
		{ID: "d", Name: "d.mflac", Status: StatusSuccess},
		{ID: "e", Name: "e.mflac", Status: StatusSkipped, Message: "输出已存在"},
	}
	summary := &Summary{Total: 5, Success: 1, Skipped: 1}
	go func() {
		_, _ = io.WriteString(h.stdoutW, "{\"t\":\"start\",\"id\":\"b\"}\n")
		_, _ = io.WriteString(h.stderrW, "Timeout was reached\n")
		_ = h.stdoutW.Close()
		_ = h.stderrW.Close()
		h.exited <- fmt.Errorf("exit status 1")
	}()

	reason := h.mustDrive(t, context.Background(), items, summary, 0)
	if reason != StopReasonTimeout {
		t.Fatalf("结束原因 = %s，期望 timeout", reason)
	}
	if summary.Cancelled {
		t.Fatal("超时不应记为用户取消")
	}
	if summary.Failed != 3 || summary.Unfinished != 3 || summary.Success != 1 || summary.Skipped != 1 {
		t.Fatalf("统计错误: %+v", summary)
	}
	if countStatus(items, StatusFailed, "解密超时，未能完成") != 3 {
		t.Fatalf("未完成文件的失败原因不正确: %+v %+v %+v", items[0], items[1], items[2])
	}
	if items[3].Status != StatusSuccess || items[3].Message != "" {
		t.Fatalf("已成功文件被改写: %+v", items[3])
	}
	if items[4].Status != StatusSkipped || items[4].Message != "输出已存在" {
		t.Fatalf("已跳过文件被改写: %+v", items[4])
	}

	notices := h.notices(t)
	if len(notices) != 1 {
		t.Fatalf("notice 数量 = %d，期望 1", len(notices))
	}
	const want = "解密已超时，注入器已退出。还有 3 个文件没有完成，已标记为失败。可以稍后重试这些文件。"
	if notices[0]["reason"] != StopReasonTimeout || notices[0]["title"] != "解密超时" || notices[0]["message"] != want || notices[0]["unfinished"] != 3 {
		t.Fatalf("通知内容不正确: %#v", notices[0])
	}
	if summary.Title != "解密超时" || summary.Message != want || summary.StopReason != StopReasonTimeout {
		t.Fatalf("摘要未带上界面文案: %+v", summary)
	}
	progress := h.finalProgress(t)
	if progress["failed"] != int64(3) || progress["done"] != int64(5) {
		t.Fatalf("最终进度不正确: %#v", progress)
	}
	if failedNotices := countFileEvents(t, h, StatusFailed, "解密超时，未能完成"); failedNotices != 3 {
		t.Fatalf("应向前端发送 3 条超时失败事件，实际 %d", failedNotices)
	}
	logs := h.logText()
	if !strings.Contains(logs, "注入器 stderr: Timeout was reached") || !strings.Contains(logs, want) {
		t.Fatalf("运行日志应保留超时原文和界面说明:\n%s", logs)
	}
}

// TestInjectorUnexpectedExitCompletesJob 覆盖注入器崩溃或其它非超时退出。
func TestInjectorUnexpectedExitCompletesJob(t *testing.T) {
	h := newSessionHarness(t)
	items := []*FileItem{
		{ID: "a", Status: StatusRunning},
		{ID: "b", Status: StatusPending},
		{ID: "c", Status: StatusSuccess},
	}
	summary := &Summary{Total: 3, Success: 1}
	go func() {
		_, _ = io.WriteString(h.stderrW, "access violation\n")
		_ = h.stdoutW.Close()
		_ = h.stderrW.Close()
		h.exited <- fmt.Errorf("exit status 0xc0000005")
	}()

	reason := h.mustDrive(t, context.Background(), items, summary, 0)
	if reason != StopReasonInjector {
		t.Fatalf("结束原因 = %s，期望 injector", reason)
	}
	if summary.Cancelled || summary.Unfinished != 2 || summary.Failed != 2 {
		t.Fatalf("统计错误: %+v", summary)
	}
	if countStatus(items, StatusFailed, "注入器意外退出，未能完成") != 2 {
		t.Fatalf("文件终态不正确: %+v %+v", items[0], items[1])
	}
	const want = "注入器意外退出，解密已中断。还有 2 个文件没有完成，已标记为失败。可以稍后重试这些文件。"
	notices := h.notices(t)
	if len(notices) != 1 || notices[0]["title"] != "注入器已退出" || notices[0]["message"] != want || notices[0]["unfinished"] != 2 {
		t.Fatalf("通知内容不正确: %#v", notices)
	}
}

// TestCancelStaysDistinctFromInjectorTimeout 确认用户取消不会被超时文案覆盖。
func TestCancelStaysDistinctFromInjectorTimeout(t *testing.T) {
	h := newSessionHarness(t)
	items := []*FileItem{
		{ID: "a", Status: StatusRunning},
		{ID: "b", Status: StatusPending},
		{ID: "c", Status: StatusSuccess},
	}
	summary := &Summary{Total: 3, Success: 1}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(40 * time.Millisecond)
		_, _ = io.WriteString(h.stderrW, "Timeout was reached\n")
		cancel()
	}()

	reason := h.mustDrive(t, ctx, items, summary, 30*time.Second)
	if reason != StopReasonCancelled || !summary.Cancelled {
		t.Fatalf("取消结果不正确: reason=%s summary=%+v", reason, summary)
	}
	if h.kills.Load() == 0 {
		t.Fatal("取消后应结束注入进程")
	}
	if countStatus(items, StatusFailed, "已取消") != 2 {
		t.Fatalf("取消文件原因不正确: %+v %+v", items[0], items[1])
	}
	if items[2].Status != StatusSuccess {
		t.Fatalf("已成功文件被取消改写: %+v", items[2])
	}
	if notices := h.notices(t); len(notices) != 0 {
		t.Fatalf("取消不应弹超时/注入器通知: %#v", notices)
	}
	if summary.StopReason != StopReasonCancelled || summary.Title != "" || summary.Message != "" {
		t.Fatalf("取消摘要不应使用超时文案: %+v", summary)
	}
}

// TestCleanFinishIgnoresTimeoutStderr 脚本已经 finished 时，不能再报超时。
func TestCleanFinishIgnoresTimeoutStderr(t *testing.T) {
	h := newSessionHarness(t)
	items := []*FileItem{
		{ID: "a", Status: StatusSuccess},
		{ID: "b", Status: StatusPending},
	}
	summary := &Summary{Total: 2, Success: 1}
	go func() {
		_, _ = io.WriteString(h.stdoutW, "{\"t\":\"finished\"}\n")
		_, _ = io.WriteString(h.stderrW, "Timeout was reached\n")
		_ = h.stdoutW.Close()
		_ = h.stderrW.Close()
		h.exited <- nil
	}()

	reason := h.mustDrive(t, context.Background(), items, summary, 0)
	if reason != StopReasonFinished || summary.Cancelled {
		t.Fatalf("正常结束被误判: reason=%s summary=%+v", reason, summary)
	}
	if items[0].Status != StatusSuccess {
		t.Fatalf("已成功文件被改写: %+v", items[0])
	}
	if items[1].Status != StatusFailed || items[1].Message != "未收到该文件的处理结果" {
		t.Fatalf("finished 之后的遗留文件终态不正确: %+v", items[1])
	}
	if notices := h.notices(t); len(notices) != 0 {
		t.Fatalf("正常结束不应发出超时通知: %#v", notices)
	}
	if h.finalProgress(t)["final"] != true {
		t.Fatal("正常结束也应发出最终进度")
	}
}

// TestInjectSessionIdleTimeoutCompletesJob 注入进程不退出、也没有 finished 时，会话超时仍要结束任务。
func TestInjectSessionIdleTimeoutCompletesJob(t *testing.T) {
	h := newSessionHarness(t)
	items := []*FileItem{
		{ID: "a", Status: StatusPending},
		{ID: "b", Status: StatusRunning},
	}
	summary := &Summary{Total: 2}

	reason := h.mustDrive(t, context.Background(), items, summary, 80*time.Millisecond)
	if reason != StopReasonTimeout {
		t.Fatalf("结束原因 = %s，期望 timeout", reason)
	}
	if h.kills.Load() == 0 {
		t.Fatal("会话超时后应结束注入进程")
	}
	if summary.Unfinished != 2 || countStatus(items, StatusFailed, "解密超时，未能完成") != 2 {
		t.Fatalf("遗留文件未按超时收口: summary=%+v items=%+v %+v", summary, items[0], items[1])
	}
	const want = "解密已超时，注入器已退出。还有 2 个文件没有完成，已标记为失败。可以稍后重试这些文件。"
	notices := h.notices(t)
	if len(notices) != 1 || notices[0]["message"] != want || notices[0]["title"] != "解密超时" {
		t.Fatalf("会话超时通知不正确: %#v", notices)
	}
	if !strings.Contains(h.logText(), "注入会话超时") {
		t.Fatalf("会话超时应写入日志:\n%s", h.logText())
	}
}

// TestScriptFatalTimeoutIsTimeout 脚本上报的加载超时要按超时通知，而不是笼统的进程退出。
func TestScriptFatalTimeoutIsTimeout(t *testing.T) {
	h := newSessionHarness(t)
	items := []*FileItem{{ID: "a", Status: StatusPending}}
	summary := &Summary{Total: 1}
	go func() {
		_, _ = io.WriteString(h.stdoutW, "{\"t\":\"fatal\",\"err\":\"等待模块 QQMusicCommon.dll 加载超时\"}\n")
	}()

	reason := h.mustDrive(t, context.Background(), items, summary, 0)
	if reason != StopReasonTimeout {
		t.Fatalf("结束原因 = %s，期望 timeout", reason)
	}
	const want = "解密已超时（等待模块 QQMusicCommon.dll 加载超时）。还有 1 个文件没有完成，已标记为失败。可以稍后重试这些文件。"
	notices := h.notices(t)
	if len(notices) != 1 || notices[0]["message"] != want {
		t.Fatalf("脚本超时通知不正确: %#v", notices)
	}
	if items[0].Status != StatusFailed || items[0].Message != "解密超时，未能完成" {
		t.Fatalf("文件原因不正确: %+v", items[0])
	}
}

// TestScriptFatalEmitsDistinctNotice 非超时的脚本致命错误使用单独文案。
func TestScriptFatalEmitsDistinctNotice(t *testing.T) {
	h := newSessionHarness(t)
	items := []*FileItem{{ID: "a", Status: StatusRunning}, {ID: "b", Status: StatusSuccess}}
	summary := &Summary{Total: 2, Success: 1}
	go func() {
		_, _ = io.WriteString(h.stdoutW, "{\"t\":\"fatal\",\"err\":\"脚本崩了\"}\n")
	}()

	reason := h.mustDrive(t, context.Background(), items, summary, 0)
	if reason != StopReasonFatal {
		t.Fatalf("结束原因 = %s，期望 fatal", reason)
	}
	const want = "注入脚本异常结束：脚本崩了。还有 1 个文件没有完成，已标记为失败。可以稍后重试这些文件。"
	notices := h.notices(t)
	if len(notices) != 1 || notices[0]["title"] != "解密中断" || notices[0]["message"] != want || notices[0]["reason"] != StopReasonFatal {
		t.Fatalf("致命错误通知不正确: %#v", notices)
	}
	if items[0].Message != "注入脚本异常结束：脚本崩了" || items[1].Status != StatusSuccess {
		t.Fatalf("文件终态不正确: %+v %+v", items[0], items[1])
	}
}

func TestClassifyStopPrecedence(t *testing.T) {
	cases := []struct {
		name            string
		userCancelled   bool
		sawFinished     bool
		sessionTimedOut bool
		fatalErr        string
		stderrText      string
		want            string
	}{
		{"cancel beats timeout text", true, false, true, "", "Timeout was reached", StopReasonCancelled},
		{"finished beats timeout text", false, true, false, "", "Timeout was reached", StopReasonFinished},
		{"stderr timeout", false, false, false, "", "Timeout was reached", StopReasonTimeout},
		{"idle session timeout", false, false, true, "", "", StopReasonTimeout},
		{"script timeout", false, false, false, "等待模块加载超时", "", StopReasonTimeout},
		{"fatal stays fatal", false, false, false, "脚本崩了", "access violation", StopReasonFatal},
		{"bare exit is injector", false, false, false, "", "", StopReasonInjector},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyStop(tc.userCancelled, tc.sawFinished, tc.sessionTimedOut, tc.fatalErr, tc.stderrText)
			if got != tc.want {
				t.Fatalf("classifyStop = %s，期望 %s", got, tc.want)
			}
		})
	}
}
