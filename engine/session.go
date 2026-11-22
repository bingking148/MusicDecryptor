package engine

import (
	"bufio"
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 任务结束原因。取消必须和超时、注入器意外退出分开。
const (
	StopReasonFinished  = "finished"
	StopReasonCancelled = "cancelled"
	StopReasonTimeout   = "timeout"
	StopReasonInjector  = "injector"
	StopReasonFatal     = "fatal"
)

// injectIdleTimeout 是注入进程仍在、但长时间没有任何脚本进度时的兜底。
// frida-inject 超时或崩溃时通常会直接退出，那条路径不依赖这个计时；
// 这里只避免会话卡住时界面一直停在「正在解密」。正在推进的文件会重置计时。
const injectIdleTimeout = 5 * time.Minute

// driveInjectorSession 读取注入器输出，直到脚本正常结束、用户取消、
// 注入进程退出，或会话因长时间没有进度而超时。
// 进程退出（包括 stderr 的 "Timeout was reached"）本身就是任务结束，
// 不能继续空等脚本的 finished / fatal。
func driveInjectorSession(
	ctx context.Context,
	stdout io.Reader,
	stderr io.Reader,
	exited <-chan error,
	kill func(),
	items []*FileItem,
	summary *Summary,
	tmpByID map[string]string,
	opts Options,
	emit Emitter,
	logf func(level, msg string),
	started time.Time,
	idleTimeout time.Duration,
) string {
	if logf == nil {
		logf = func(string, string) {}
	}
	if emit == nil {
		emit = func(string, any) {}
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var mu sync.Mutex
	var fatalErr string
	sawFinished := atomic.Bool{}
	sessionTimedOut := atomic.Bool{}

	poke := func() {}
	if idleTimeout > 0 {
		activity := make(chan struct{}, 1)
		poke = func() {
			select {
			case activity <- struct{}{}:
			default:
			}
		}
		go func() {
			timer := time.NewTimer(idleTimeout)
			defer timer.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-activity:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(idleTimeout)
				case <-timer.C:
					sessionTimedOut.Store(true)
					logf("warn", "注入会话超时：长时间没有新的解密进度")
					cancel()
					return
				}
			}
		}()
	}

	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 256*1024), 8*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			poke()
			msg := parseScriptMessage(line)
			if msg == nil {
				logf("debug", line)
				continue
			}
			id, _ := msg["id"].(string)
			switch msg["t"] {
			case "start":
				mu.Lock()
				setItemStatus(items, id, StatusRunning, "")
				snap := snapshotItem(items, id)
				mu.Unlock()
				if snap != nil {
					emit("file", snap)
				}
			case "done":
				tmp := tmpByID[id]
				mu.Lock()
				errMsg := finalizeItem(items, id, tmp, opts)
				if errMsg != "" {
					setItemStatus(items, id, StatusFailed, errMsg)
					summary.Failed++
				} else {
					setItemStatus(items, id, StatusSuccess, "")
					summary.Success++
				}
				snap := snapshotItem(items, id)
				mu.Unlock()
				os.Remove(tmp)
				if snap != nil {
					emit("file", snap)
				}
				emitProgress(emit, summary, started, false)
			case "fail":
				os.Remove(tmpByID[id])
				reason, _ := msg["err"].(string)
				mu.Lock()
				setItemStatus(items, id, StatusFailed, reason)
				summary.Failed++
				snap := snapshotItem(items, id)
				mu.Unlock()
				if snap != nil {
					emit("file", snap)
				}
				emitProgress(emit, summary, started, false)
				logf("warn", "失败："+id+"："+reason)
			case "fatal":
				reason, _ := msg["err"].(string)
				fatalErr = strings.TrimSpace(reason)
				logf("error", "注入脚本致命错误："+fatalErr)
				cancel()
			case "finished":
				sawFinished.Store(true)
				cancel()
			}
		}
	}()

	var stderrText string
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		data, _ := io.ReadAll(stderr)
		stderrText = strings.TrimSpace(string(data))
		if stderrText != "" {
			logf("warn", "注入器 stderr: "+stderrText)
		}
	}()

	exitOnce := make(chan struct{})
	go func() {
		defer close(exitOnce)
		if exited != nil {
			<-exited
		}
	}()

	// finished / fatal / 用户取消会关掉 runCtx。
	// 注入器超时、崩溃或其他非正常退出往往没有这些事件，只是进程结束。
	// 标准输出提前结束同样要收口，避免界面继续停在「正在解密」。
	select {
	case <-runCtx.Done():
	case <-exitOnce:
		cancel()
	case <-stdoutDone:
		select {
		case <-exitOnce:
		case <-runCtx.Done():
		case <-time.After(2 * time.Second):
		}
		cancel()
	}

	var killOnce sync.Once
	if kill != nil {
		killOnce.Do(kill)
	}
	<-exitOnce
	<-stdoutDone
	<-stderrDone

	reason := classifyStop(ctx.Err() != nil, sawFinished.Load(), sessionTimedOut.Load(), fatalErr, stderrText)
	title, message, fileReason := failureCopy(reason, 0, fatalErr)

	mu.Lock()
	var leftovers []FileItem
	for _, it := range items {
		if it.Status != StatusPending && it.Status != StatusRunning {
			continue
		}
		it.Status = StatusFailed
		it.Message = fileReason
		summary.Failed++
		leftovers = append(leftovers, *it)
	}
	unfinished := len(leftovers)
	mu.Unlock()

	for i := range leftovers {
		emit("file", &leftovers[i])
	}

	title, message, _ = failureCopy(reason, unfinished, fatalErr)
	summary.StopReason = reason
	summary.Cancelled = reason == StopReasonCancelled
	summary.Unfinished = int64(unfinished)
	summary.ElapsedMs = time.Since(started).Milliseconds()
	if reason == StopReasonTimeout || reason == StopReasonInjector || reason == StopReasonFatal {
		summary.Title = title
		summary.Message = message
	}
	// 先刷新进度数字，再发通知，对话框出现时失败数已经是终态。
	emitProgress(emit, summary, started, true)
	if summary.Message != "" && (reason == StopReasonTimeout || reason == StopReasonInjector || reason == StopReasonFatal) {
		emit("notice", map[string]any{
			"reason":     reason,
			"title":      title,
			"message":    message,
			"unfinished": unfinished,
		})
		logf("error", message)
	}
	return reason
}

func classifyStop(userCancelled, sawFinished, sessionTimedOut bool, fatalErr, stderrText string) string {
	if userCancelled {
		return StopReasonCancelled
	}
	if sawFinished {
		return StopReasonFinished
	}
	if sessionTimedOut || isTimeoutText(fatalErr) || isTimeoutText(stderrText) {
		return StopReasonTimeout
	}
	if strings.TrimSpace(fatalErr) != "" {
		return StopReasonFatal
	}
	return StopReasonInjector
}

func isTimeoutText(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	if strings.Contains(s, "超时") {
		return true
	}
	lower := strings.ToLower(s)
	return strings.Contains(lower, "timeout was reached") || strings.Contains(lower, "timed out")
}

// failureCopy 生成主界面和文件行使用的简体中文说明。
// unfinished 只统计这次被收成终态的待处理/进行中文件，不含更早已经失败的文件。
func failureCopy(reason string, unfinished int, fatalErr string) (title, message, fileReason string) {
	detail := strings.Join(strings.Fields(fatalErr), " ")
	switch reason {
	case StopReasonCancelled:
		return "", "", "已取消"
	case StopReasonFinished:
		return "", "", "未收到该文件的处理结果"
	case StopReasonTimeout:
		title = "解密超时"
		fileReason = "解密超时，未能完成"
		if unfinished > 0 {
			message = "解密已超时，注入器已退出。还有 " + strconv.Itoa(unfinished) + " 个文件没有完成，已标记为失败。可以稍后重试这些文件。"
			if detail != "" {
				message = "解密已超时（" + detail + "）。还有 " + strconv.Itoa(unfinished) + " 个文件没有完成，已标记为失败。可以稍后重试这些文件。"
			}
		} else {
			message = "解密已超时，注入器已退出。当前文件都已经有结果。"
		}
	case StopReasonInjector:
		title = "注入器已退出"
		fileReason = "注入器意外退出，未能完成"
		if unfinished > 0 {
			message = "注入器意外退出，解密已中断。还有 " + strconv.Itoa(unfinished) + " 个文件没有完成，已标记为失败。可以稍后重试这些文件。"
		} else {
			message = "注入器意外退出，解密已中断。当前文件都已经有结果。"
		}
	default:
		title = "解密中断"
		shown := detail
		if shown == "" {
			shown = "未知错误"
		}
		fileReason = "注入脚本异常结束：" + shown
		if unfinished > 0 {
			message = "注入脚本异常结束：" + shown + "。还有 " + strconv.Itoa(unfinished) + " 个文件没有完成，已标记为失败。可以稍后重试这些文件。"
		} else {
			message = "注入脚本异常结束：" + shown + "。当前文件都已经有结果。"
		}
	}
	return title, message, fileReason
}
