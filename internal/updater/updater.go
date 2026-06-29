package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GetCurrentVersion 获取当前版本号
func GetCurrentVersion() string {
	return "v1.0.3"
}

// CleanupTemp 清理更新遗留的临时文件
func CleanupTemp() {
	// 清理历史自更新功能可能遗留的临时目录
	tempDir := filepath.Join(os.TempDir(), "kiro-update")
	os.RemoveAll(tempDir)

	// 清理历史自更新功能可能遗留的 .backup 文件
	if exePath, err := os.Executable(); err == nil {
		backupPath := exePath + ".backup"
		os.Remove(backupPath)
	}
}

// githubRelease GitHub Release API 响应
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
}

const githubReleasesURL = "https://api.github.com/repos/huey1in/kirox/releases/latest"

// semverGreater 返回 a 是否语义上大于 b（格式 vX.Y.Z 或 X.Y.Z）
func semverGreater(a, b string) bool {
	parse := func(v string) [3]int {
		v = strings.TrimPrefix(v, "v")
		parts := strings.SplitN(v, ".", 3)
		var nums [3]int
		for i, p := range parts {
			if i >= 3 {
				break
			}
			fmt.Sscanf(p, "%d", &nums[i])
		}
		return nums
	}
	va, vb := parse(a), parse(b)
	for i := 0; i < 3; i++ {
		if va[i] != vb[i] {
			return va[i] > vb[i]
		}
	}
	return false
}

// CheckUpdate 检查 GitHub 最新 Release。
// 当前更新入口只展示版本信息并跳转 Release 页面，不再在应用内下载/替换 exe。
func CheckUpdate() map[string]interface{} {
	currentVersion := GetCurrentVersion()

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", githubReleasesURL, nil)
	if err != nil {
		return map[string]interface{}{"error": "构建请求失败: " + err.Error()}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kirox/"+currentVersion)

	resp, err := client.Do(req)
	if err != nil {
		return map[string]interface{}{"error": "检查更新失败: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return map[string]interface{}{
			"hasUpdate":      false,
			"currentVersion": currentVersion,
			"latestVersion":  currentVersion,
			"message":        "暂无发布版本",
		}
	}
	if resp.StatusCode != 200 {
		return map[string]interface{}{"error": fmt.Sprintf("GitHub API 返回 %d", resp.StatusCode)}
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return map[string]interface{}{"error": "解析响应失败: " + err.Error()}
	}

	latestVersion := release.TagName
	if latestVersion == "" {
		latestVersion = release.Name
	}

	hasUpdate := latestVersion != "" && semverGreater(latestVersion, currentVersion)

	releaseDate := ""
	if len(release.PublishedAt) >= 10 {
		releaseDate = release.PublishedAt[:10]
	}

	releaseURL := release.HTMLURL
	if releaseURL == "" {
		releaseURL = "https://github.com/huey1in/kirox/releases/latest"
	}

	return map[string]interface{}{
		"hasUpdate":      hasUpdate,
		"currentVersion": currentVersion,
		"latestVersion":  latestVersion,
		"releaseDate":    releaseDate,
		"changelog":      release.Body,
		"releaseURL":     releaseURL,
	}
}
