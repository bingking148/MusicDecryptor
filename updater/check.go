package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	repoReleasesURL = "https://api.github.com/repos/bingking148/MusicDecryptor/releases?per_page=20"
	maxReleaseBody  = 2 << 20
)

// Result 是一次更新检查的结论。Available 为真时才提供应用内安装。
type Result struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Available      bool   `json:"available"`
	AssetName      string `json:"assetName,omitempty"`
	AssetURL       string `json:"assetURL,omitempty"`
	AssetSize      int64  `json:"assetSize,omitempty"`
	Notes          string `json:"notes,omitempty"`
	PageURL        string `json:"pageURL,omitempty"`
	AssetMissing   bool   `json:"assetMissing"`
	Message        string `json:"message,omitempty"`
}

// Release 是比较所需的发布信息。
type Release struct {
	Tag        string
	Draft      bool
	Prerelease bool
	Body       string
	PageURL    string
	Assets     []Asset
}

// Asset 是发布中的一个文件。
type Asset struct {
	Name string
	URL  string
	Size int64
}

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Body       string        `json:"body"`
	HTMLURL    string        `json:"html_url"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// SelectUpdate 从发布列表里决定要不要提示更新。
// 草稿和预发布版本不参与比较。没有更新、或新版本缺少 Windows 安装包时不返回错误。
func SelectUpdate(current string, releases []Release) (Result, error) {
	cur, err := ParseVersion(current)
	if err != nil {
		return Result{}, fmt.Errorf("当前版本号无效：%s", current)
	}
	result := Result{CurrentVersion: strings.TrimSpace(current), Message: "已是最新版本"}
	var best *Release
	var bestVer Version
	for i := range releases {
		rel := &releases[i]
		if rel.Draft || rel.Prerelease {
			continue
		}
		ver, err := ParseVersion(rel.Tag)
		if err != nil {
			continue
		}
		if best == nil || ver.Compare(bestVer) > 0 {
			best = rel
			bestVer = ver
		}
	}
	if best == nil {
		result.Message = "没有已发布的版本"
		return result, nil
	}
	result.LatestVersion = strings.TrimSpace(best.Tag)
	result.Notes = strings.TrimSpace(best.Body)
	result.PageURL = best.PageURL
	if bestVer.Compare(cur) <= 0 {
		result.Message = "已是最新版本"
		return result, nil
	}
	asset, ok := selectWindowsAsset(best.Assets)
	if !ok {
		result.AssetMissing = true
		result.Message = fmt.Sprintf("发现新版本 %s，但发布中没有可用的 Windows 安装包", result.LatestVersion)
		return result, nil
	}
	result.Available = true
	result.AssetName = asset.Name
	result.AssetURL = asset.URL
	result.AssetSize = asset.Size
	result.Message = fmt.Sprintf("可以安装 %s。下载完成后会替换当前程序并自动重启。", result.LatestVersion)
	return result, nil
}

func selectWindowsAsset(assets []Asset) (Asset, bool) {
	matches := make([]Asset, 0, 1)
	for _, asset := range assets {
		if windowsAMD64Exe(asset) {
			matches = append(matches, asset)
		}
	}
	if len(matches) != 1 {
		return Asset{}, false
	}
	return matches[0], true
}

func windowsAMD64Exe(asset Asset) bool {
	name := strings.ToLower(strings.TrimSpace(asset.Name))
	if asset.URL == "" || asset.Size <= 0 || !strings.HasSuffix(name, ".exe") {
		return false
	}
	if !strings.Contains(name, "windows") || !strings.Contains(name, "amd64") {
		return false
	}
	if strings.Contains(name, "ia32") || strings.Contains(name, "arm64") || strings.Contains(name, "386") {
		return false
	}
	return true
}

// Check 读取 GitHub Releases，并和当前版本比较。
func (c *Client) Check(ctx context.Context, current string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.releasesURL(), nil)
	if err != nil {
		return Result{}, fmt.Errorf("检查更新失败：无法连接 GitHub")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "MusicDecryptor/"+CurrentVersion)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("检查更新失败：无法连接 GitHub")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseBody+1))
	if err != nil {
		return Result{}, fmt.Errorf("检查更新失败：无法读取服务器响应")
	}
	if len(body) > maxReleaseBody {
		return Result{}, fmt.Errorf("检查更新失败：服务器返回了无法识别的内容")
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("检查更新失败：GitHub 返回 %d", resp.StatusCode)
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return Result{}, fmt.Errorf("检查更新失败：服务器返回了空内容")
	}
	if trimmed[0] != '[' {
		return Result{}, fmt.Errorf("检查更新失败：服务器返回了无法识别的内容")
	}
	var payload []githubRelease
	if err := json.Unmarshal(body, &payload); err != nil {
		return Result{}, fmt.Errorf("检查更新失败：服务器返回了无法识别的内容")
	}
	releases := make([]Release, 0, len(payload))
	for _, item := range payload {
		assets := make([]Asset, 0, len(item.Assets))
		for _, asset := range item.Assets {
			assets = append(assets, Asset{Name: asset.Name, URL: asset.BrowserDownloadURL, Size: asset.Size})
		}
		releases = append(releases, Release{
			Tag:        item.TagName,
			Draft:      item.Draft,
			Prerelease: item.Prerelease,
			Body:       item.Body,
			PageURL:    item.HTMLURL,
			Assets:     assets,
		})
	}
	return SelectUpdate(current, releases)
}
