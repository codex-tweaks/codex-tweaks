package core

import (
	"strings"
	"testing"
)

func TestInjectionCreatesStylesBeforeExecutingPackages(t *testing.T) {
	payload := Payload{Version: "order", Packages: []CompiledPackage{
		{ID: "first", Name: "first", Version: "1.0.0", CSS: "first-css", JavaScript: "first-js"},
		{ID: "second", Name: "second", Version: "1.0.0", CSS: "second-css", JavaScript: "second-js"},
	}}
	script := InjectionScript(payload, 0)
	firstCSS, secondCSS := strings.Index(script, "first-css"), strings.Index(script, "second-css")
	firstJS, secondJS := strings.Index(script, "first-js"), strings.Index(script, "second-js")
	if !(firstCSS >= 0 && firstCSS < secondCSS && secondCSS < firstJS && firstJS < secondJS) {
		t.Fatalf("unexpected setup/execution order: %d %d %d %d", firstCSS, secondCSS, firstJS, secondJS)
	}
}

func TestInjectionRuntimeAndForceContracts(t *testing.T) {
	pkg := CompiledPackage{ID: "sample", Name: "sample", Version: "1.0.0", CSS: "body::before { content: \"</style>\\n你好\"; }", JavaScript: "module.exports.activate = ({ root }) => { root.textContent = `ok`; };"}
	script := InjectionScript(Payload{Version: "base", Packages: []CompiledPackage{pkg}}, 7)
	for _, expected := range []string{
		`"base-force-7"`, "__CODEX_TWEAKS__", `host.style.display = "contents"`,
		"registerCleanup(callback)", "registerLibrary(name, value)", "Package dependency unavailable",
		"Package entry must export activate(context)", "cleanupPackageState(state)", "packageStates.delete(packageID)",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("script missing %q", expected)
		}
	}
	if !strings.Contains(CleanupScript, "delete globalThis[key]") || !strings.Contains(CleanupScript, "codex-tweaks-root") {
		t.Fatal("cleanup contract changed")
	}
}

func TestInjectionExposesTypedNodeAndSettingsExtensions(t *testing.T) {
	payload := Payload{Version: "renderer-extensions", Packages: []CompiledPackage{{
		ID: "sample", Name: "sample", Version: "1.0.0",
		UI: CompiledPackageUI{SettingsSections: &CompiledSettingsSections{
			Required: false,
			Items: []RuntimeSettingsSection{{
				PackageID: "sample", ID: "wallpaper", Title: "自定义背景",
				Slug: "codex-tweaks-sample-wallpaper-12345678",
			}},
		}},
		Node: &CompiledPackageNode{
			AuthorizationID: "authorization", Reason: "读取背景图片。",
		},
		JavaScript: `module.exports.activate = ({ node, ui }) => node.invoke("ping", ui.settingsSections.list());`,
	}}}
	configuration := &SettingsAdapterConfiguration{
		AppModuleURL:        "app://-/assets/app-initial-test.js",
		VisibilityModuleURL: "app://-/assets/use-visible-settings-sections-test.js",
		Sections: []RuntimeSettingsSection{{
			PackageID: "sample", ID: "wallpaper", Title: "自定义背景",
			Slug: "codex-tweaks-sample-wallpaper-12345678",
		}},
	}
	script := injectionScriptWithRendererBridge(
		payload, 0, "bridge-session", map[string]string{"sample": "secret-token"}, configuration,
	)
	for _, expected := range []string{
		`"bridge-session"`, `"secret-token"`, `"appModuleUrl"`,
		"createPackageNode(", "createPackageUI(", "ui.settingsSections",
		"settleNodeInvocation", "emitNodeEvent",
		"registry.push({ slug: descriptor.slug })", "iconMap[descriptor.slug]",
		"settingsRouteChildren.splice(", "dispatchHostMessage", "labelRegistry", "groupRegistry",
		`key.startsWith("__reactFiber$")`, "settingsAdapter?.cleanup?.()",
		"nodePendingLimit = 64",
		"settingsAdapterReady",
		"try {\n      const context = {",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("renderer extension injection script missing %q", expected)
		}
	}
}
