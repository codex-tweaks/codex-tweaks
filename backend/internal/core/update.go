package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	UpdateRepository    = "cr-zhichen/codex-tweaks"
	UpdateRepositoryURL = "https://github.com/" + UpdateRepository
)

type UpdateChannel string

const (
	UpdateStable UpdateChannel = "stable"
	UpdateBeta   UpdateChannel = "beta"
)

type GitHubAsset struct {
	Name               string  `json:"name"`
	BrowserDownloadURL *string `json:"browser_download_url,omitempty"`
}

type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	PublishedAt *CodableTime  `json:"published_at,omitempty"`
	HTMLURL     *string       `json:"html_url,omitempty"`
	Assets      []GitHubAsset `json:"assets"`
}

type UpdateService struct {
	httpClient *http.Client
}

func NewUpdateService(httpClient *http.Client) *UpdateService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &UpdateService{httpClient: httpClient}
}

func (s *UpdateService) Check(ctx context.Context, channel UpdateChannel, currentVersion string) (*GitHubRelease, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+UpdateRepository+"/releases?per_page=100", nil)
	if err != nil {
		return nil, errors.New("更新地址无效。")
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	request.Header.Set("User-Agent", "Codex-Tweaks/"+currentVersion)
	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case http.StatusForbidden:
			return nil, errors.New("GitHub 暂时限制了更新请求，请稍后再试。")
		case http.StatusNotFound:
			return nil, errors.New("没有找到 Codex Tweaks 的 Release 仓库。")
		default:
			return nil, fmt.Errorf("检查更新失败（HTTP %d）。", response.StatusCode)
		}
	}
	var releases []GitHubRelease
	if err := json.NewDecoder(response.Body).Decode(&releases); err != nil {
		return nil, errors.New("GitHub 返回了无法识别的响应。")
	}
	return SelectLatestRelease(releases, channel), nil
}

func SelectLatestRelease(releases []GitHubRelease, channel UpdateChannel) *GitHubRelease {
	type candidate struct {
		Release GitHubRelease
		Version SemanticVersion
	}
	candidates := []candidate{}
	for _, release := range releases {
		version, ok := ParseSemanticVersion(release.TagName)
		if !ok || release.Draft {
			continue
		}
		matches := false
		switch channel {
		case UpdateBeta:
			matches = release.Prerelease && version.BetaOrRC() || !release.Prerelease && version.Stable()
		default:
			matches = !release.Prerelease && version.Stable()
		}
		if matches {
			candidates = append(candidates, candidate{Release: release, Version: version})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(left, right int) bool { return candidates[left].Version.Compare(candidates[right].Version) < 0 })
	selected := candidates[len(candidates)-1].Release
	return &selected
}

func PreferredDownloadURL(release GitHubRelease, operatingSystem, architecture string) *string {
	if operatingSystem == "" {
		operatingSystem = runtime.GOOS
	}
	if architecture == "" {
		architecture = runtime.GOARCH
	}
	switch operatingSystem {
	case "darwin":
		return preferredAssetURL(release, ".dmg", architecture)
	case "windows":
		if hasAssetSuffix(release, ".msi") {
			return preferredAssetURL(release, ".msi", architecture)
		}
		return preferredAssetURL(release, ".exe", architecture)
	default:
		return release.HTMLURL
	}
}

func preferredAssetURL(release GitHubRelease, extension, architecture string) *string {
	assets := []GitHubAsset{}
	for _, asset := range release.Assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), extension) {
			assets = append(assets, asset)
		}
	}
	architectureSuffix := "-x86_64" + extension
	if architecture == "arm64" {
		architectureSuffix = "-arm64" + extension
	}
	for _, asset := range assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), architectureSuffix) {
			return assetURL(asset, release.HTMLURL)
		}
	}
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if !strings.HasSuffix(name, "-arm64"+extension) && !strings.HasSuffix(name, "-x86_64"+extension) {
			return assetURL(asset, release.HTMLURL)
		}
	}
	if len(assets) > 0 {
		return assetURL(assets[0], release.HTMLURL)
	}
	return release.HTMLURL
}

func assetURL(asset GitHubAsset, fallback *string) *string {
	if asset.BrowserDownloadURL != nil {
		return asset.BrowserDownloadURL
	}
	return fallback
}

func hasAssetSuffix(release GitHubRelease, suffix string) bool {
	for _, asset := range release.Assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), suffix) {
			return true
		}
	}
	return false
}

func HasNewerVersion(release *GitHubRelease, installedVersion string) bool {
	if release == nil {
		return false
	}
	latest, latestOK := ParseSemanticVersion(release.TagName)
	installed, installedOK := ParseSemanticVersion(installedVersion)
	return latestOK && installedOK && latest.Compare(installed) > 0
}

func packageChannelForRelease(release *GitHubRelease) UpdateChannel {
	if release != nil && release.Prerelease {
		return UpdateBeta
	}
	return UpdateStable
}
