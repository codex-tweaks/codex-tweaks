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

func TestInjectionExposesGrantedCapabilitiesAndSettingsAdapter(t *testing.T) {
	payload := Payload{Version: "capabilities", Packages: []CompiledPackage{{
		ID: "sample", Name: "sample", Version: "1.0.0",
		Capabilities: map[string]GrantedCapability{
			NetworkCapabilityID: {Version: NetworkCapabilityVersion},
			UISettingsSectionCapabilityID: {
				Version:     UISettingsSectionCapabilityVersion,
				Permissions: []byte(`{"sections":[{"id":"wallpaper","title":"随机背景","slug":"codex-tweaks-sample-wallpaper-12345678"}]}`),
			},
		},
		JavaScript: `module.exports.activate = ({ capabilities }) => capabilities.require("network");`,
	}}}
	configuration := &SettingsAdapterConfiguration{
		AppModuleURL:        "app://-/assets/app-initial-test.js",
		VisibilityModuleURL: "app://-/assets/use-visible-settings-sections-test.js",
		Sections: []RuntimeSettingsSection{{
			PackageID: "sample",
			UISettingsSectionPermission: UISettingsSectionPermission{
				ID: "wallpaper", Title: "随机背景", Slug: "codex-tweaks-sample-wallpaper-12345678",
			},
		}},
	}
	script := injectionScriptWithCapabilities(
		payload, 0, "bridge-session", map[string]string{"sample": "secret-token"}, configuration,
	)
	for _, expected := range []string{
		`"bridge-session"`, `"secret-token"`, `"appModuleUrl"`,
		"capabilities: createPackageCapabilities(", "ui.settings-section",
		"registry.push({ slug: descriptor.slug })", "iconMap[descriptor.slug]",
		"settingsRouteChildren.splice(", "dispatchHostMessage", "labelRegistry", "groupRegistry",
		`key.startsWith("__reactFiber$")`, "settingsAdapter?.cleanup?.()",
		"capabilityPendingLimit = 64",
		"settingsAdapterReady",
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("capability injection script missing %q", expected)
		}
	}
}
