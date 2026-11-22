package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxAssetSize int64 = 200 << 20

// Client 访问 GitHub。测试可以换成本地服务器。
type Client struct {
	HTTP        *http.Client
	ReleasesURL string
	AllowLocal  bool
}

// DefaultClient 只接受 GitHub 发布地址。
func DefaultClient() *Client {
	return &Client{ReleasesURL: repoReleasesURL}
}

func (c *Client) releasesURL() string {
	if c != nil && c.ReleasesURL != "" {
		return c.ReleasesURL
	}
	return repoReleasesURL
}

func (c *Client) httpClient() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{
		Timeout:       20 * time.Minute,
		CheckRedirect: secureRedirect,
	}
}

func secureRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("重定向次数过多")
	}
	if !AllowedAssetURL(req.URL.String()) {
		return fmt.Errorf("下载失败：安装包地址不可用")
	}
	return nil
}

// AllowedAssetURL 判断正式更新是否来自 GitHub 发布域名。
func AllowedAssetURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return false
	}
	return githubReleaseHost(u.Hostname())
}

func githubReleaseHost(host string) bool {
	host = strings.ToLower(host)
	switch host {
	case "github.com", "api.github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com":
		return true
	default:
		return strings.HasSuffix(host, ".githubusercontent.com")
	}
}

func (c *Client) assetURLAllowed(raw string) bool {
	if AllowedAssetURL(raw) {
		return true
	}
	if c == nil || !c.AllowLocal {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// Download 把安装包写到 dest，并核对大小和 PE 头。
func (c *Client) Download(ctx context.Context, assetURL string, expectedSize int64, dest string, progress func(received, total int64)) error {
	if expectedSize <= 0 || expectedSize > maxAssetSize {
		return fmt.Errorf("下载失败：安装包大小异常")
	}
	if !c.assetURLAllowed(assetURL) {
		return fmt.Errorf("下载失败：安装包地址不可用")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return fmt.Errorf("下载失败：安装包地址不可用")
	}
	req.Header.Set("User-Agent", "MusicDecryptor/"+CurrentVersion)
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("下载失败：无法连接下载地址")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败：服务器返回 %d", resp.StatusCode)
	}
	file, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("下载失败：无法保存安装包")
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	var received int64
	var header [2]byte
	headerLen := 0
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if received+int64(n) > expectedSize {
				return fmt.Errorf("下载失败：文件大小与发布信息不符")
			}
			if headerLen < 2 {
				take := 2 - headerLen
				if take > n {
					take = n
				}
				copy(header[headerLen:], buf[:take])
				headerLen += take
			}
			written, err := file.Write(buf[:n])
			received += int64(written)
			if err != nil {
				return fmt.Errorf("下载失败：无法保存安装包")
			}
			if progress != nil {
				progress(received, expectedSize)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
				return fmt.Errorf("下载失败：连接中断")
			}
			var netErr net.Error
			if errors.As(readErr, &netErr) {
				return fmt.Errorf("下载失败：连接中断")
			}
			return fmt.Errorf("下载失败：无法读取安装包")
		}
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("下载失败：无法保存安装包")
	}
	if received != expectedSize || headerLen < 2 || header[0] != 'M' || header[1] != 'Z' {
		return fmt.Errorf("下载失败：安装包内容异常")
	}
	return nil
}
