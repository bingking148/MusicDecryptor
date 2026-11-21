package engine

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	PhoneBridgePort    = 47321
	phoneBridgeMaxBody = 256 << 20
	phoneCodeTTL       = 15 * time.Minute
	phoneLockAfter     = 5
	phoneLockFor       = 2 * time.Minute
)

// FileDecryptor 解密一个已落在临时目录里的加密文件，返回产物路径。
// 产物必须仍在调用方提供的文件所在任务目录内。
type FileDecryptor func(ctx context.Context, srcPath string) (string, error)

// BridgeInfo 是桌面端展示给用户的接力状态。配对码只在本机界面出现。
type BridgeInfo struct {
	Enabled   bool     `json:"enabled"`
	Port      int      `json:"port"`
	Addrs     []string `json:"addrs"`
	Code      string   `json:"code"`
	ExpiresAt string   `json:"expiresAt"`
	Message   string   `json:"message"`
}

// PhoneBridge 是默认关闭的局域网接力。只监听回环和私网地址，不监听公网网卡。
type PhoneBridge struct {
	Decrypt  FileDecryptor
	TempRoot string
	MaxBytes int64

	mu         sync.Mutex
	servers    []*http.Server
	code       string
	codePretty string
	codeUntil  time.Time
	token      string
	fails      int
	lockUntil  time.Time
	port       int
	addrs      []string
}

func (b *PhoneBridge) Start() (BridgeInfo, error) {
	return b.start(PhoneBridgePort)
}

func (b *PhoneBridge) start(prefer int) (BridgeInfo, error) {
	if b.Decrypt == nil {
		return BridgeInfo{}, errors.New("未配置解密器")
	}
	b.Stop()

	port := prefer
	if port <= 0 {
		probe, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return BridgeInfo{}, err
		}
		port = probe.Addr().(*net.TCPAddr).Port
		probe.Close()
	}
	var listeners []net.Listener
	var addrs []string
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		listeners, addrs, lastErr = listenPrivate(port)
		if lastErr == nil && len(listeners) > 0 {
			break
		}
		port++
	}
	if len(listeners) == 0 {
		if lastErr == nil {
			lastErr = errors.New("没有可监听的私网地址")
		}
		return BridgeInfo{}, lastErr
	}

	code, pretty := newPairCode()
	root := b.TempRoot
	if root == "" {
		root = os.TempDir()
	}
	maxBytes := b.MaxBytes
	if maxBytes <= 0 {
		maxBytes = phoneBridgeMaxBody
	}

	b.mu.Lock()
	b.code = code
	b.codePretty = pretty
	b.codeUntil = time.Now().Add(phoneCodeTTL)
	b.token = ""
	b.fails = 0
	b.lockUntil = time.Time{}
	b.port = port
	b.addrs = addrs
	b.mu.Unlock()

	handler := b.routes(maxBytes, root)
	b.mu.Lock()
	b.servers = nil
	for _, ln := range listeners {
		srv := &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		}
		srv.SetKeepAlivesEnabled(false)
		b.servers = append(b.servers, srv)
		go srv.Serve(ln)
	}
	info := b.snapshotLocked()
	b.mu.Unlock()
	return info, nil
}

func (b *PhoneBridge) Stop() {
	b.mu.Lock()
	servers := b.servers
	b.servers = nil
	b.token = ""
	b.code = ""
	b.codePretty = ""
	b.addrs = nil
	b.mu.Unlock()
	for _, srv := range servers {
		_ = srv.Close()
	}
}

func (b *PhoneBridge) Status() BridgeInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.snapshotLocked()
}

func (b *PhoneBridge) snapshotLocked() BridgeInfo {
	info := BridgeInfo{
		Enabled: len(b.servers) > 0,
		Port:    b.port,
		Addrs:   append([]string(nil), b.addrs...),
		Code:    b.codePretty,
		Message: "未开启。开启后只在这台电脑的局域网地址上等待手机配对。",
	}
	if !b.codeUntil.IsZero() {
		info.ExpiresAt = b.codeUntil.Format(time.RFC3339)
	}
	if info.Enabled {
		info.Message = "已开启。把地址和配对码填进手机；用完请关闭。不要在公共网络开启。"
	}
	return info
}

func (b *PhoneBridge) routes(maxBytes int64, tempRoot string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeBridgeError(w, http.StatusMethodNotAllowed, "只接受 GET")
			return
		}
		writeBridgeJSON(w, http.StatusOK, map[string]any{
			"service": "musicdecryptor-phone",
			"version": 1,
		})
	})
	mux.HandleFunc("/v1/pair", b.handlePair)
	mux.HandleFunc("/v1/decrypt", func(w http.ResponseWriter, r *http.Request) {
		b.handleDecrypt(w, r, maxBytes, tempRoot)
	})
	return mux
}

func (b *PhoneBridge) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeBridgeError(w, http.StatusMethodNotAllowed, "只接受 POST")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBridgeError(w, http.StatusBadRequest, "配对请求无法读取")
		return
	}
	got := normalizeCode(req.Code)

	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.lockUntil.IsZero() && time.Now().Before(b.lockUntil) {
		writeBridgeError(w, http.StatusTooManyRequests, "配对尝试过多，请稍后再试")
		return
	}
	if b.code == "" || time.Now().After(b.codeUntil) {
		writeBridgeError(w, http.StatusUnauthorized, "配对码已失效，请在电脑上重新开启")
		return
	}
	if len(got) != len(b.code) || subtle.ConstantTimeCompare([]byte(got), []byte(b.code)) != 1 {
		b.fails++
		if b.fails >= phoneLockAfter {
			b.lockUntil = time.Now().Add(phoneLockFor)
			b.fails = 0
		}
		writeBridgeError(w, http.StatusUnauthorized, "配对码不正确")
		return
	}
	b.fails = 0
	b.token = randomHex(32)
	writeBridgeJSON(w, http.StatusOK, map[string]any{"token": b.token})
}

func (b *PhoneBridge) handleDecrypt(w http.ResponseWriter, r *http.Request, maxBytes int64, tempRoot string) {
	if r.Method != http.MethodPost {
		writeBridgeError(w, http.StatusMethodNotAllowed, "只接受 POST")
		return
	}
	if !b.authorized(r.Header.Get("Authorization")) {
		writeBridgeError(w, http.StatusUnauthorized, "未配对或配对已失效")
		return
	}
	name, err := safeUploadName(r.Header.Get("X-Filename"))
	if err != nil {
		writeBridgeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.ContentLength < 0 {
		writeBridgeError(w, http.StatusLengthRequired, "需要声明文件大小")
		return
	}
	if r.ContentLength > maxBytes {
		writeBridgeError(w, http.StatusRequestEntityTooLarge, "文件超过 256MB，已拒绝")
		return
	}

	dir, err := os.MkdirTemp(tempRoot, "phone-")
	if err != nil {
		writeBridgeError(w, http.StatusInternalServerError, "无法创建临时目录")
		return
	}
	defer os.RemoveAll(dir)
	inDir := filepath.Join(dir, "in")
	if err := os.MkdirAll(inDir, 0o700); err != nil {
		writeBridgeError(w, http.StatusInternalServerError, "无法创建临时目录")
		return
	}
	dst := filepath.Join(inDir, name)
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		writeBridgeError(w, http.StatusInternalServerError, "无法保存上传文件")
		return
	}
	written, copyErr := io.Copy(f, io.LimitReader(r.Body, maxBytes+1))
	closeErr := f.Close()
	if copyErr != nil {
		if r.Context().Err() != nil {
			return
		}
		writeBridgeError(w, http.StatusBadRequest, "读取上传内容失败")
		return
	}
	if closeErr != nil || written > maxBytes {
		writeBridgeError(w, http.StatusRequestEntityTooLarge, "文件超过 256MB，已拒绝")
		return
	}

	outPath, err := b.Decrypt(r.Context(), dst)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		writeBridgeError(w, http.StatusConflict, err.Error())
		return
	}
	if !pathInside(dir, outPath) {
		writeBridgeError(w, http.StatusInternalServerError, "解密产物路径无效")
		return
	}
	rf, err := os.Open(outPath)
	if err != nil {
		writeBridgeError(w, http.StatusConflict, "解密产物不存在")
		return
	}
	defer rf.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Output-Name", filepath.Base(outPath))
	w.Header().Set("X-Audio-Ext", strings.ToLower(filepath.Ext(outPath)))
	_, _ = io.Copy(w, rf)
}

func (b *PhoneBridge) authorized(header string) bool {
	const prefix = "Bearer "
	if len(header) < len(prefix) || header[:len(prefix)] != prefix {
		return false
	}
	got := header[len(prefix):]
	b.mu.Lock()
	token := b.token
	b.mu.Unlock()
	if token == "" || len(got) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func listenPrivate(port int) ([]net.Listener, []string, error) {
	ips := []net.IP{net.IPv4(127, 0, 0, 1)}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{"127.0.0.1": true}
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || !isPrivateIPv4(ip) || seen[ip.String()] {
				continue
			}
			seen[ip.String()] = true
			ips = append(ips, ip)
		}
	}

	var listeners []net.Listener
	var shown []string
	var loopback net.Listener
	for _, ip := range ips {
		ln, err := net.Listen("tcp", net.JoinHostPort(ip.String(), fmt.Sprint(port)))
		if err != nil {
			if ip.IsLoopback() {
				return nil, nil, err
			}
			continue
		}
		if ip.IsLoopback() {
			loopback = ln
			continue
		}
		listeners = append(listeners, ln)
		shown = append(shown, ln.Addr().String())
	}
	if loopback == nil {
		for _, ln := range listeners {
			ln.Close()
		}
		return nil, nil, errors.New("无法监听本机回环地址")
	}
	listeners = append([]net.Listener{loopback}, listeners...)
	return listeners, shown, nil
}

func isPrivateIPv4(ip net.IP) bool {
	if ip[0] == 10 || (ip[0] == 192 && ip[1] == 168) || (ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) {
		return true
	}
	return false
}

func newPairCode() (raw, pretty string) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, 8)
	rand.Read(buf)
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	raw = string(buf)
	return raw, raw[:4] + "-" + raw[4:]
}

func normalizeCode(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return strings.ReplaceAll(s, "-", "")
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func safeUploadName(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
		return "", errors.New("文件名无效")
	}
	lower := strings.ToLower(name)
	if !allowedPhoneExt(lower) {
		return "", errors.New("只接受 .mflac / .mgg / .qmc 文件")
	}
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '|', '?', '*':
			return '_'
		default:
			return r
		}
	}, name)
	return cleaned, nil
}

func allowedPhoneExt(lower string) bool {
	for _, ext := range []string{".qmcflac", ".qmcogg", ".qmc0", ".qmc2", ".qmc3", ".qmc4", ".qmc6", ".qmc8"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	for _, base := range []string{".mflac", ".mgg"} {
		if i := strings.LastIndex(lower, base); i >= 0 {
			rest := lower[i+len(base):]
			if rest == "" {
				return true
			}
			ok := true
			for _, r := range rest {
				if r < '0' || r > '9' {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
	}
	return false
}

func pathInside(root, target string) bool {
	root, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeBridgeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeBridgeError(w http.ResponseWriter, status int, msg string) {
	writeBridgeJSON(w, status, map[string]string{"error": msg})
}
