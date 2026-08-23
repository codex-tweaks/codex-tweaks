package core

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var scpRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:[^\s]+$`)

func ValidateSource(source PackageSource) error {
	value := strings.TrimSpace(source.URL)
	if !scpRepositoryPattern.MatchString(value) {
		parsed, err := url.Parse(value)
		hasPassword := false
		if parsed != nil && parsed.User != nil {
			_, hasPassword = parsed.User.Password()
		}
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "ssh") || hasPassword || parsed.Scheme == "https" && parsed.User != nil {
			return errors.New("Git 仓库地址无效；仅支持 HTTPS 或 SSH，且不能在 URL 中包含凭据。")
		}
	}
	if selectorNeedsValue(source.Selector.Type) && source.Selector.Value == nil {
		return errors.New("远程来源缺少选择值。")
	}
	if (source.Selector.Type == SelectorGitHubLatestRelease || source.Selector.Type == SelectorGitHubRelease) && !isGitHubRepository(value) {
		return errors.New("GitHub Release 选择器要求 github.com 仓库地址。")
	}
	switch source.Selector.Type {
	case SelectorBranch, SelectorLatestSemverTag, SelectorTag, SelectorGitHubLatestRelease, SelectorGitHubRelease, SelectorCommit:
		return nil
	default:
		return errors.New("不支持的远程来源选择器。")
	}
}

func selectorNeedsValue(selectorType RemoteSelectorType) bool {
	switch selectorType {
	case SelectorBranch, SelectorTag, SelectorGitHubRelease, SelectorCommit:
		return true
	default:
		return false
	}
}

func isGitHubRepository(raw string) bool {
	value := strings.TrimSpace(raw)
	var repositoryPath string
	if strings.HasPrefix(value, "git@github.com:") {
		repositoryPath = strings.TrimPrefix(value, "git@github.com:")
	} else {
		parsed, err := url.Parse(value)
		if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
			return false
		}
		repositoryPath = strings.Trim(parsed.Path, "/")
	}
	repositoryPath = strings.TrimSuffix(repositoryPath, ".git")
	parts := strings.Split(repositoryPath, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}
