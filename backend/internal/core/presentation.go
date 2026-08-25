package core

import (
	"runtime"
	"strconv"
	"strings"
)

const PresentationContractVersion = 1

type PresentationTokens struct {
	WindowMinWidth      int    `json:"windowMinWidth"`
	WindowMinHeight     int    `json:"windowMinHeight"`
	WindowDefaultWidth  int    `json:"windowDefaultWidth"`
	WindowDefaultHeight int    `json:"windowDefaultHeight"`
	NavigationWidth     int    `json:"navigationWidth"`
	ContentMaxWidth     int    `json:"contentMaxWidth"`
	PagePadding         int    `json:"pagePadding"`
	SectionSpacing      int    `json:"sectionSpacing"`
	CardPadding         int    `json:"cardPadding"`
	CardCornerRadius    int    `json:"cardCornerRadius"`
	ControlSpacing      int    `json:"controlSpacing"`
	CompactSpacing      int    `json:"compactSpacing"`
	StatusIconSize      int    `json:"statusIconSize"`
	AnimationFastMS     int    `json:"animationFastMS"`
	AnimationStandardMS int    `json:"animationStandardMS"`
	AccentColor         string `json:"accentColor"`
	SuccessColor        string `json:"successColor"`
	WarningColor        string `json:"warningColor"`
	DangerColor         string `json:"dangerColor"`
}

type AvailableActions struct {
	OpenCodex                  bool `json:"openCodex"`
	RestartCodex               bool `json:"restartCodex"`
	RestartCodexUI             bool `json:"restartCodexUI"`
	Reinject                   bool `json:"reinject"`
	OpenPackagesDirectory      bool `json:"openPackagesDirectory"`
	OpenLogFile                bool `json:"openLogFile"`
	OpenRepository             bool `json:"openRepository"`
	SetEnabled                 bool `json:"setEnabled"`
	SetDeveloperMode           bool `json:"setDeveloperMode"`
	ReloadPackages             bool `json:"reloadPackages"`
	InstallLocalPackage        bool `json:"installLocalPackage"`
	InstallRemotePackage       bool `json:"installRemotePackage"`
	CheckNodeEnvironment       bool `json:"checkNodeEnvironment"`
	CheckGitEnvironment        bool `json:"checkGitEnvironment"`
	CheckManagedPackageUpdates bool `json:"checkManagedPackageUpdates"`
	RefreshLog                 bool `json:"refreshLog"`
	ClearLog                   bool `json:"clearLog"`
	ReadAuthoringPrompt        bool `json:"readAuthoringPrompt"`
	CheckAppUpdate             bool `json:"checkAppUpdate"`
	SetUpdatePreferences       bool `json:"setUpdatePreferences"`
	InstallAppUpdate           bool `json:"installAppUpdate"`
}

type PlatformPresentation struct {
	OperatingSystem       string `json:"operatingSystem"`
	Architecture          string `json:"architecture"`
	CDPEndpoint           string `json:"cdpEndpoint"`
	RepositoryURL         string `json:"repositoryURL"`
	UpdateInstallStrategy string `json:"updateInstallStrategy"`
}

type StatusPresentation struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Tone   string `json:"tone"`
}

type PresentationContract struct {
	Version  int                  `json:"version"`
	Locale   string               `json:"locale"`
	Text     map[string]string    `json:"text"`
	Tokens   PresentationTokens   `json:"tokens"`
	Actions  AvailableActions     `json:"actions"`
	Status   StatusPresentation   `json:"status"`
	Platform PlatformPresentation `json:"platform"`
}

type PresentationState struct {
	Status                   AppStatus
	Enabled                  bool
	RestartingCodexUI        bool
	CheckingNode             bool
	CheckingGit              bool
	CheckingRemoteUpdates    bool
	InstallingLocalPackage   bool
	InstallingRemotePackage  bool
	ExportingPackage         bool
	GitAvailable             bool
	LogAvailable             bool
	AuthoringPromptAvailable bool
	UpdateChecking           bool
	UpdateAvailable          bool
}

func NewPresentationContract(state PresentationState) PresentationContract {
	return NewPresentationContractForPlatform(state, runtime.GOOS, runtime.GOARCH)
}

func NewPresentationContractForPlatform(state PresentationState, operatingSystem, architecture string) PresentationContract {
	text := PresentationText()
	cdpAvailable := state.Status.Kind == StatusWaitingForPage || state.Status.Kind == StatusConnected || state.Status.Kind == StatusDisabled
	uiRestartAvailable := state.Status.Kind == StatusConnected || state.Status.Kind == StatusDisabled || state.Status.Kind == StatusError
	strategy := "openDownload"
	if operatingSystem == "darwin" {
		strategy = "sparkle"
	} else if operatingSystem == "windows" {
		strategy = "velopack"
	}
	return PresentationContract{
		Version: PresentationContractVersion,
		Locale:  "zh-CN",
		Text:    text,
		Tokens:  PresentationTokensForPlatform(operatingSystem),
		Actions: AvailableActions{
			OpenCodex:                  true,
			RestartCodex:               state.Status.Kind == StatusRestartRequired,
			RestartCodexUI:             uiRestartAvailable && !state.RestartingCodexUI,
			Reinject:                   state.Enabled && cdpAvailable && !state.RestartingCodexUI,
			OpenPackagesDirectory:      true,
			OpenLogFile:                true,
			OpenRepository:             true,
			SetEnabled:                 true,
			SetDeveloperMode:           true,
			ReloadPackages:             true,
			InstallLocalPackage:        !state.InstallingLocalPackage && !state.InstallingRemotePackage && !state.ExportingPackage,
			InstallRemotePackage:       state.GitAvailable && !state.InstallingRemotePackage && !state.InstallingLocalPackage && !state.ExportingPackage,
			CheckNodeEnvironment:       !state.CheckingNode,
			CheckGitEnvironment:        !state.CheckingGit,
			CheckManagedPackageUpdates: state.GitAvailable && !state.CheckingRemoteUpdates,
			RefreshLog:                 true,
			ClearLog:                   state.LogAvailable,
			ReadAuthoringPrompt:        state.AuthoringPromptAvailable,
			CheckAppUpdate:             !state.UpdateChecking,
			SetUpdatePreferences:       !state.UpdateChecking,
			InstallAppUpdate:           operatingSystem == "windows" || state.UpdateAvailable,
		},
		Status: statusPresentation(state.Status, text),
		Platform: PlatformPresentation{
			OperatingSystem:       operatingSystem,
			Architecture:          architecture,
			CDPEndpoint:           CodexCDPEndpoint,
			RepositoryURL:         UpdateRepositoryURL,
			UpdateInstallStrategy: strategy,
		},
	}
}

func statusPresentation(status AppStatus, text map[string]string) StatusPresentation {
	result := StatusPresentation{
		Title:  text["status.starting.title"],
		Detail: text["overview.connectingDetail"],
		Tone:   "accent",
	}
	switch status.Kind {
	case StatusLaunchingCodex:
		result.Title = text["status.launchingCodex.title"]
	case StatusCodexNotRunning:
		result.Title = text["status.codexNotRunning.title"]
		result.Detail = text["status.codexNotRunning.detail"]
		result.Tone = "neutral"
	case StatusWaitingForCDP:
		result.Title = text["status.waitingForCDP.title"]
	case StatusRestartRequired:
		result.Title = text["status.restartRequired.title"]
		result.Detail = text["status.restartRequired.detail"]
		result.Tone = "warning"
	case StatusWaitingForPage:
		result.Title = text["status.waitingForPage.title"]
		result.Detail = text["status.waitingForPage.detail"]
	case StatusConnected:
		if status.TargetCount == 1 {
			result.Title = text["status.connected.one"]
		} else {
			result.Title = resolvePresentationText(text, "status.connected.many", map[string]string{
				"count": strconv.Itoa(status.TargetCount),
			})
		}
		result.Detail = text["overview.connectedDetail"]
		result.Tone = "success"
	case StatusDisabled:
		result.Title = text["status.disabled.title"]
		result.Detail = text["overview.disabledDetail"]
		result.Tone = "neutral"
	case StatusError:
		result.Title = text["status.error.title"]
		result.Detail = text["overview.errorDetail"]
		result.Tone = "danger"
	}
	if status.Message != nil && strings.TrimSpace(*status.Message) != "" {
		result.Detail = *status.Message
	}
	return result
}

func resolvePresentationText(text map[string]string, key string, replacements map[string]string) string {
	value := text[key]
	for name, replacement := range replacements {
		value = strings.ReplaceAll(value, "{"+name+"}", replacement)
	}
	return value
}

func PresentationTokensForPlatform(operatingSystem string) PresentationTokens {
	if strings.EqualFold(operatingSystem, "windows") {
		return PresentationTokens{
			WindowMinWidth: 1120, WindowMinHeight: 800, WindowDefaultWidth: 1320, WindowDefaultHeight: 920,
			NavigationWidth: 220, ContentMaxWidth: 1080,
			PagePadding: 28, SectionSpacing: 20, CardPadding: 18, CardCornerRadius: 12,
			ControlSpacing: 10, CompactSpacing: 6, StatusIconSize: 20,
			AnimationFastMS: 120, AnimationStandardMS: 220,
			AccentColor: "#0A84FF", SuccessColor: "#30D158", WarningColor: "#FF9F0A", DangerColor: "#FF453A",
		}
	}

	return PresentationTokens{
		WindowMinWidth: 820, WindowMinHeight: 560, WindowDefaultWidth: 920, WindowDefaultHeight: 640,
		NavigationWidth: 220, ContentMaxWidth: 1120,
		PagePadding: 32, SectionSpacing: 28, CardPadding: 20, CardCornerRadius: 14,
		ControlSpacing: 12, CompactSpacing: 7, StatusIconSize: 36,
		AnimationFastMS: 120, AnimationStandardMS: 220,
		AccentColor: "#0A84FF", SuccessColor: "#30D158", WarningColor: "#FF9F0A", DangerColor: "#FF453A",
	}
}

func PresentationText() map[string]string {
	return map[string]string{
		"app.name":                                    "Codex Tweaks",
		"app.backendMissing":                          "应用目录中缺少 Go 后端可执行文件。",
		"app.backendNotRunning":                       "Go 后端尚未运行。",
		"app.backendTerminated":                       "Go 后端已退出（状态码 {status}）。",
		"app.backendMalformed":                        "Go 后端返回了无法解析的响应。",
		"app.backendDateMalformed":                    "无法解析 Go 后端返回的日期：{value}",
		"app.backendRequestFailed":                    "Go 后端请求失败。",
		"app.backendRequestCreateFailed":              "无法创建 Go 后端请求。",
		"app.protocolMismatch":                        "Go 后端协议版本不匹配。",
		"nav.overview":                                "概览",
		"nav.packages":                                "功能包",
		"nav.logs":                                    "运行日志",
		"nav.updates":                                 "关于与更新",
		"status.starting.title":                       "正在启动",
		"status.launchingCodex.title":                 "正在启动 Codex",
		"status.codexNotRunning.title":                "Codex 未运行",
		"status.waitingForCDP.title":                  "正在等待调试端口",
		"status.restartRequired.title":                "Codex 需要重启",
		"status.waitingForPage.title":                 "正在等待 Codex 页面",
		"status.connected.one":                        "已连接 Codex",
		"status.connected.many":                       "已连接 {count} 个窗口",
		"status.disabled.title":                       "界面增强已停用",
		"status.error.title":                          "连接异常",
		"status.restartRequired.detail":               "当前 Codex 未开启本地 CDP 端口",
		"status.waitingForPage.detail":                "调试端口可用，尚未发现 app:// 页面",
		"status.codexNotRunning.detail":               "可以重新打开 Codex",
		"overview.title":                              "管理 Codex 的本地界面增强",
		"overview.subtitle":                           "连接状态、注入控制与常用入口集中在一个窗口中。",
		"overview.control":                            "控制",
		"overview.enable":                             "启用界面增强",
		"overview.enableDetail":                       "停用后会清理已注入的样式、组件和事件监听器。",
		"overview.reinject":                           "重新注入",
		"overview.restartCodexUI":                     "重启 Codex 界面",
		"overview.restartCodexUIDetail":               "只重新加载界面，不退出 Codex；界面被功能包卡住时可用来恢复。",
		"overview.managePackages":                     "管理功能包",
		"overview.viewLogs":                           "查看日志",
		"overview.aiAuthoring":                        "交给 AI 编写",
		"overview.copySkill":                          "复制 Codex Tweaks 功能包开发 Skill",
		"overview.copySkillDetail":                    "复制内容直接来自项目内统一的 SKILL.md，与功能包协议、依赖和验证约束始终使用同一份来源。粘贴后再追加具体需求。",
		"overview.copyHint":                           "复制后可粘贴给 Codex 或其他智能体",
		"overview.copy":                               "复制 Skill",
		"overview.copied":                             "已复制",
		"overview.connection":                         "连接方式",
		"overview.cdpEndpoint":                        "CDP 端点",
		"overview.injectionScope":                     "注入范围",
		"overview.appPagesOnly":                       "仅 app:// 页面",
		"overview.refreshPolicy":                      "刷新策略",
		"overview.refreshEveryTwoSeconds":             "每 2 秒检查功能包与窗口",
		"overview.loadOrder":                          "包加载顺序",
		"overview.loadOrderDetail":                    "依赖拓扑优先，再按用户有效优先级从小到大",
		"overview.resources":                          "资源目录",
		"overview.openPackagesDirectory":              "在文件管理器中管理功能包",
		"overview.openCodex":                          "打开 Codex",
		"overview.restartAndConnect":                  "重启并连接",
		"overview.connectedDetail":                    "已编译且启用的功能包已应用到 Codex。",
		"overview.disabledDetail":                     "Codex 保持运行，但不会应用任何自定义内容。",
		"overview.connectingDetail":                   "Codex Tweaks 正在建立本地连接。",
		"overview.openCodexDetail":                    "打开 Codex 后会自动建立连接。",
		"overview.restartDetail":                      "需要重新启动 Codex 才能开启本地调试端口。",
		"overview.errorDetail":                        "请查看运行日志了解详细原因。",
		"packages.title":                              "管理页面增强",
		"packages.subtitle":                           "每个目录是一个独立包。源码更新不会直接生效，手动编译成功后才会原子切换。",
		"packages.developerMode":                      "开发者模式",
		"packages.developerModeDetail":                "开发者模式会自动编译已启用包的源码变化；依赖或版本更新仍需手动确认。",
		"packages.rescan":                             "重新扫描",
		"packages.installLocal":                       "安装本地包",
		"packages.installing":                         "正在安装…",
		"packages.installLocalHelp":                   "选择功能包目录或 ZIP；校验通过后复制到本地 packages 目录",
		"packages.installRemote":                      "从 Git 安装",
		"packages.checkRemoteUpdates":                 "检查远程更新",
		"packages.checkingRemote":                     "正在检查…",
		"packages.enabledSummary":                     "已启用 {enabled} / {total}，已激活 {active}",
		"packages.chooseZip":                          "选择 ZIP 文件",
		"packages.chooseDirectory":                    "选择功能包目录",
		"packages.nodeChecking":                       "正在检测 Node.js…",
		"packages.nodeAvailable":                      "Node.js {version} 可用",
		"packages.nodeMissing":                        "未找到 Node.js、npm 和 npx；安装 Node.js 后重新扫描。",
		"packages.gitChecking":                        "正在检测 Git…",
		"packages.gitAvailable":                       "{version} 可用",
		"packages.gitMissing":                         "未找到 Git；本地功能包仍可使用，但不能安装或检查远程包。",
		"packages.installingLocal":                    "正在安全检查并安装本地功能包…",
		"packages.clearMessage":                       "清除操作提示",
		"packages.clearError":                         "清除操作错误",
		"packages.exportZip":                          "导出为 ZIP",
		"packages.exportZipHelp":                      "将功能包源码保存为可再次安装的 ZIP",
		"packages.exporting":                          "正在导出 {name}…",
		"packages.zipFileType":                        "ZIP 压缩包",
		"packages.exportSuccess":                      "已导出 {name}：{file}",
		"packages.exportFailed":                       "导出 {name} 失败：{message}",
		"packages.enablePackage":                      "启用 {name}",
		"packages.expanded":                           "已展开",
		"packages.collapsed":                          "已折叠",
		"packages.loadOrder":                          "加载顺序",
		"packages.loadOrderDetail":                    "依赖拓扑优先；其余优先级数值越低越先加载",
		"packages.searchPlaceholder":                  "搜索功能包",
		"packages.filter":                             "筛选功能包",
		"packages.filter.all":                         "全部",
		"packages.filter.enabled":                     "已启用",
		"packages.filter.disabled":                    "已停用",
		"packages.filter.pending":                     "待处理",
		"packages.filter.error":                       "异常",
		"packages.sourceVersion":                      "源 v{version}",
		"packages.activeVersion":                      "已激活 v{version}",
		"packages.installDependencies":                "安装缺失依赖",
		"packages.enableDependencies":                 "启用依赖",
		"packages.updateAndBuild":                     "更新并编译",
		"packages.openDirectory":                      "在文件管理器中打开功能包",
		"packages.priority":                           "优先级",
		"packages.userPriority":                       "{name} 用户优先级",
		"packages.resetPriority":                      "恢复包默认优先级 {priority}",
		"packages.dependencyConstraint":               "依赖约束",
		"packages.dependencyOrderFirst":               "依赖顺序优先",
		"packages.priorityActual":                     "实际加载顺序 #{position}。用户优先级 {priority} 仍用于和没有依赖路径的功能包排序。",
		"packages.priorityAfter":                      "{name} 依赖于 {dependencies}，因此必须在这些包之后加载。",
		"packages.priorityBefore":                     "{dependencies} 依赖于 {name}，因此当前包必须先于这些包加载。",
		"packages.packageCountSuffix":                 " 等 {count} 个包",
		"packages.emptyTitle":                         "没有找到功能包",
		"packages.emptyDetail":                        "在 packages 目录中创建包目录和 package.json 后重新扫描。",
		"packages.noMatchTitle":                       "没有匹配的功能包",
		"packages.noMatchDetail":                      "尝试更换关键词或筛选条件。",
		"packages.clearSearch":                        "清除搜索与筛选",
		"packages.build":                              "编译",
		"packages.rebuild":                            "重新编译",
		"packages.building":                           "编译中…",
		"packages.cannotBuild":                        "无法编译",
		"packages.notBuiltDetail":                     "点击编译后才会加载到页面。",
		"packages.noDescription":                      "没有提供包说明。",
		"packages.status.active":                      "已激活",
		"packages.status.pending":                     "待编译",
		"packages.status.invalid":                     "配置无效",
		"packages.status.disabled":                    "已停用",
		"packages.status.dependency":                  "依赖待处理",
		"packages.status.remoteUpdate":                "远程有更新",
		"packages.status.installingRemote":            "正在处理远程包",
		"packages.status.building":                    "正在编译",
		"packages.status.exporting":                   "正在导出",
		"packages.status.remoteFailed":                "远程操作失败",
		"packages.status.pinnedChanged":               "固定引用已变化",
		"packages.status.dependencyBlocked":           "依赖阻塞",
		"packages.status.buildFailed":                 "编译失败",
		"packages.status.payloadFailed":               "产物读取失败",
		"packages.status.runtimeFailed":               "运行失败",
		"packages.status.unavailable":                 "不可用",
		"packages.status.notBuilt":                    "尚未编译",
		"packages.status.newVersion":                  "发现新版本",
		"packages.status.dependencyChanged":           "依赖或配置更新",
		"packages.status.sourceChanged":               "源码有更新",
		"packages.status.compilerChanged":             "编译器有更新",
		"packages.detail.remoteAvailable":             "{reference} 已更新到 {commit}，点击后下载并编译。",
		"packages.detail.pinnedChanged":               "远端固定 Tag/Release 指向了新的 commit；为避免静默替换，已阻止普通更新。",
		"packages.detail.dependencyIssues":            "有 {count} 个依赖问题，展开依赖详情可查看具体状态。",
		"packages.detail.activeBuildError":            "当前仍运行 v{version}。{message}",
		"packages.detail.versionUpdate":               "当前仍运行 v{current}，点击后更新到 v{next}。",
		"packages.detail.dependencyUpdate":            "当前编译产物仍在运行；需手动同步依赖或构建配置。",
		"packages.detail.sourceChanged":               "当前编译产物仍在运行，新源码尚未激活。",
		"packages.detail.compilerUpdate":              "当前产物由 esbuild {version} 生成。",
		"packages.detail.lastBuilt":                   "上次编译：{date}",
		"packages.detail.exporting":                   "正在将功能包源码整理为可再次安装的 ZIP。",
		"packages.installAndBuild":                    "安装并编译",
		"packages.updateToVersion":                    "更新到 v{version}",
		"packages.syncAndBuild":                       "同步并编译",
		"packages.updateBuild":                        "更新编译",
		"packages.dependencyNoGitSource":              "未声明 Git 来源，仅在本地查找",
		"packages.dependencyInstalledSource":          "本机已安装来源：{url}",
		"packages.dependencySummaryOK":                "依赖 {total}/{total} 正常",
		"packages.dependencySummaryCritical":          "依赖 {total} · {pending} 个异常",
		"packages.dependencySummaryPending":           "依赖 {total} · {pending} 个需处理",
		"packages.dependencyState.satisfied":          "正常",
		"packages.dependencyState.satisfiedVersion":   "正常 · v{version}",
		"packages.dependencyState.missingLocal":       "本地缺失",
		"packages.dependencyState.missingInstallable": "可从 Git 安装",
		"packages.dependencyState.disabled":           "已停用",
		"packages.dependencyState.disabledVersion":    "已停用 · v{version}",
		"packages.dependencyState.notBuilt":           "尚未编译",
		"packages.dependencyState.notBuiltVersion":    "尚未编译 · v{version}",
		"packages.dependencyState.versionMismatch":    "v{version} 不匹配",
		"packages.dependencyState.sourceConflict":     "来源冲突",
		"packages.dependencyState.cycle":              "循环依赖",
		"packages.dependencyState.blocked":            "依赖链阻塞",
		"packages.dependencyState.invalidRequirement": "版本范围无效",
		"packages.dependencyState.selfReference":      "依赖自身",
		"packages.dependencyOrigin.local":             "本地提供",
		"packages.dependencyOrigin.managed":           "Git 管理",
		"packages.dependencyOrigin.localOnly":         "仅本地",
		"packages.dependencyOrigin.installable":       "Git 可安装",
		"packages.dependencySelector":                 "{type}：{value}",
		"remote.title":                                "从 Git 安装功能包",
		"remote.subtitle":                             "程序会解析远程引用、锁定精确 commit，并在临时目录校验包格式后保存不可变源码。",
		"remote.repository":                           "仓库地址",
		"remote.repositoryPlaceholder":                "https://github.com/owner/package.git",
		"remote.selector":                             "版本选择",
		"remote.install":                              "安装",
		"remote.close":                                "关闭",
		"remote.validationDetail":                     "安装会校验 package.json、API 版本、SemVer、入口、包依赖与 npm 锁文件。通过后新包默认保持停用；有 Node.js 时会继续下载锁定依赖并编译，但不会自动启用。",
		"selector.branch":                             "指定分支",
		"selector.latestSemverTag":                    "最新 SemVer Tag",
		"selector.tag":                                "指定 Tag",
		"selector.githubLatestRelease":                "GitHub 最新 Release",
		"selector.githubRelease":                      "指定 GitHub Release",
		"selector.commit":                             "指定 Commit",
		"selector.branchValue":                        "分支名称",
		"selector.tagValue":                           "Tag 名称",
		"selector.githubReleaseValue":                 "Release 的 Tag",
		"selector.commitValue":                        "Commit SHA",
		"selector.branchDefault":                      "main",
		"logs.title":                                  "连接与注入日志",
		"logs.subtitle":                               "查看最近的启动、连接和注入日志；完整内容可打开日志文件。",
		"logs.refresh":                                "刷新",
		"logs.openFile":                               "打开日志文件",
		"logs.clear":                                  "清除日志",
		"logs.clearTitle":                             "清除所有日志？",
		"logs.clearMessage":                           "日志文件中的现有记录将被永久删除，此操作无法撤销。",
		"logs.clearFailed":                            "无法清除日志",
		"logs.empty":                                  "暂无日志",
		"common.cancel":                               "取消",
		"common.confirm":                              "确认",
		"common.ok":                                   "好",
		"common.close":                                "关闭",
		"common.copy":                                 "复制",
		"common.open":                                 "打开",
		"update.title":                                "关于与更新",
		"update.subtitle":                             "选择更新通道，并从 GitHub Releases 获取适合当前设备的版本。",
		"update.versionBuild":                         "版本 {version}（构建 {build}）",
		"update.repository":                           "项目主页",
		"update.softwareUpdate":                       "软件更新",
		"update.channel":                              "更新通道",
		"update.channel.stable":                       "正式版",
		"update.channel.beta":                         "测试版",
		"update.channel.stableDetail":                 "仅检查正式发布版本",
		"update.channel.betaDetail":                   "检查正式版、Beta 与 RC",
		"update.currentVersion":                       "当前版本",
		"update.latestVersion":                        "通道最新版本",
		"update.lastCheck":                            "上次检查",
		"update.never":                                "从未",
		"update.autoCheck":                            "自动检查更新；发现新版本时询问，确认后下载并安装",
		"update.noRelease":                            "当前通道还没有可用的 GitHub Release。",
		"update.check":                                "检查更新",
		"update.download":                             "下载 {version}",
		"update.install":                              "安装 {version}",
		"update.restoreReminder":                      "恢复提醒",
		"update.viewRelease":                          "查看 Release",
		"update.available":                            "有新版本",
		"update.checking":                             "正在检查…",
		"update.skipped":                              "已跳过",
		"update.current":                              "已是最新",
		"update.skipMessage":                          "已跳过版本 {version}，仍可手动下载或恢复提醒。",
		"update.later":                                "稍后",
		"update.skip":                                 "跳过此版本",
		"update.promptMessage":                        "当前版本为 {current}，新版本 {latest} 已可用。是否现在下载并安装？",
		"update.applyProgress":                        "正在下载并安装完整更新…",
		"update.downloadProgress":                     "正在下载更新：{progress}%",
		"update.installingProgress":                   "下载完成，正在安装并重启…",
		"update.notInstalled":                         "当前是便携构建；请先使用 Setup.exe 安装后再使用自动更新。",
		"update.installFailed":                        "安装更新失败：{message}",
		"update.checkFailed":                          "检查更新失败：{message}",
		"update.checkFirst":                           "请先检查更新。",
		"update.noneAvailable":                        "当前没有可安装的更新。",
		"menu.show":                                   "显示 Codex Tweaks",
		"menu.quit":                                   "退出 Codex Tweaks",
		"dialog.restartTitle":                         "重新启动 Codex？",
		"dialog.restartMessage":                       "Codex 只有在启动时才能开启 CDP。重启后 Codex Tweaks 会自动恢复注入。",
		"dialog.restartCodexUITitle":                  "重启 Codex 界面？",
		"dialog.restartCodexUIMessage":                "这会重新加载所有已连接的 Codex 界面，但不会退出 Codex 主进程。未提交的输入可能丢失。",
	}
}
