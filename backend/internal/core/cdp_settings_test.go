package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSettingsModuleAssetDiscovery(t *testing.T) {
	appSource := `const page=()=>import(` + "`" + `./settings-page-ABC_123.js` + "`" + `);`
	settingsSource := `import{n as e}from"./use-visible-settings-sections-DEF-456.js";`
	if got := matchAsset(settingsPageImportPattern, appSource); got != "./settings-page-ABC_123.js" {
		t.Fatalf("settings page asset = %q", got)
	}
	if got := visibilityAssetPattern.FindString(settingsSource); got != "./use-visible-settings-sections-DEF-456.js" {
		t.Fatalf("visibility asset = %q", got)
	}
	if got := matchAsset(settingsPageImportPattern, "const settings = [];"); got != "" {
		t.Fatalf("unexpected asset = %q", got)
	}
}

func TestSettingsModuleAssetFixtures(t *testing.T) {
	appPath := os.Getenv("CODEX_TWEAKS_APP_INITIAL_FIXTURE")
	settingsPath := os.Getenv("CODEX_TWEAKS_SETTINGS_PAGE_FIXTURE")
	if appPath == "" || settingsPath == "" {
		t.Skip("set app-initial and settings-page fixture paths for a Codex compatibility check")
	}
	appSource, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatal(err)
	}
	settingsSource, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if matchAsset(settingsPageImportPattern, string(appSource)) == "" ||
		visibilityAssetPattern.FindString(string(settingsSource)) == "" {
		t.Fatal("current Codex module assets were not discovered")
	}
}

func TestSettingsAdapterLiveCDP(t *testing.T) {
	if os.Getenv("CODEX_TWEAKS_LIVE_CDP") != "1" {
		t.Skip("set CODEX_TWEAKS_LIVE_CDP=1 for a live in-memory compatibility check")
	}
	service := NewCDPService(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	targets, err := service.discoverTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	permissions, err := normalizeUISettingsSectionPermissions(json.RawMessage(
		`{"sections":[{"id":"compatibility-test","title":"Compatibility Test"}]}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	bound, err := bindUISettingsSectionPermissions("compatibility-test", permissions)
	if err != nil {
		t.Fatal(err)
	}
	payload := Payload{Packages: []CompiledPackage{{
		ID: "compatibility-test",
		Capabilities: map[string]GrantedCapability{
			UISettingsSectionCapabilityID: {
				Version: UISettingsSectionCapabilityVersion, Permissions: bound,
			},
		},
	}}}
	errorsByTarget := []string{}
	for _, target := range targets {
		if strings.Contains(target.URL, "initialRoute=") {
			continue
		}
		session, openError := openCapabilitySession(
			ctx, service.dialer, service.AllowedOrigin, target, service.broker, nil,
		)
		if openError != nil {
			errorsByTarget = append(errorsByTarget, target.ID+": "+openError.Error())
			continue
		}
		configuration, adapterError := session.ensureSettingsAdapter(ctx, payload)
		session.Close()
		if adapterError == nil && configuration != nil {
			return
		}
		if adapterError != nil {
			errorsByTarget = append(errorsByTarget, target.ID+": "+adapterError.Error())
		}
	}
	t.Fatalf("no live Codex target accepted ui.settings-section@1.0.0: %s", strings.Join(errorsByTarget, "; "))
}

func TestSettingsAdapterLiveInjection(t *testing.T) {
	if os.Getenv("CODEX_TWEAKS_LIVE_SETTINGS_INJECTION") != "1" {
		t.Skip("set CODEX_TWEAKS_LIVE_SETTINGS_INJECTION=1 for a visible live settings check")
	}
	service := NewCDPService(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	targets, err := service.discoverTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var target *CDPTarget
	for index := range targets {
		if !strings.Contains(targets[index].URL, "initialRoute=") {
			target = &targets[index]
			break
		}
	}
	if target == nil {
		t.Fatal("no main Codex target")
	}
	original, err := service.evaluate(ctx, `(() => ({
      settingsSlug: document.querySelector(
        'button[data-settings-panel-slug][aria-current="page"]'
      )?.dataset?.settingsPanelSlug ?? ""
    }))()`, *target.WebSocketDebuggerURL)
	if err != nil {
		t.Fatal(err)
	}
	originalSettingsSlug, _ := original["settingsSlug"].(string)
	appModuleURL := ""
	openedCustom := false
	restoreNavigation := func(restoreContext context.Context) {
		if !openedCustom || appModuleURL == "" {
			return
		}
		restoreExpression := fmt.Sprintf(`(async () => {
          const appModule = await import(%s);
          const bus = Object.values(appModule).find((value) =>
            value && typeof value === "object"
            && value.handlers instanceof Map
            && typeof value.dispatchHostMessage === "function"
          );
          if (!bus) return { restored: false };
          %s
          return { restored: true };
        })()`, JSONLiteral(appModuleURL), func() string {
			if originalSettingsSlug != "" {
				return fmt.Sprintf(`bus.dispatchHostMessage({
              type: "navigate-to-route",
              path: "/settings/" + %s,
              replace: true
            });`, JSONLiteral(originalSettingsSlug))
			}
			return `bus.dispatchHostMessage({ type: "navigate-back" });`
		}())
		_, _ = service.evaluate(restoreContext, restoreExpression, *target.WebSocketDebuggerURL)
		openedCustom = false
	}
	defer func() {
		_, _ = service.evaluate(context.Background(), CleanupScript, *target.WebSocketDebuggerURL)
		restoreNavigation(context.Background())
		_, _ = service.evaluate(context.Background(), `(() => {
          if (globalThis.__CODEX_TWEAKS_UI_LIVE_ERRORS__?.originalConsoleError) {
            console.error = globalThis.__CODEX_TWEAKS_UI_LIVE_ERRORS__.originalConsoleError;
          }
          delete globalThis.__CODEX_TWEAKS_UI_LIVE_TEST__;
          delete globalThis.__CODEX_TWEAKS_UI_LIVE_ERRORS__;
          delete globalThis.__CODEX_TWEAKS_SETTINGS_ROUTE_ELEMENT__;
          delete globalThis.__CODEX_TWEAKS_ROUTER_CANDIDATE__;
          return { restored: true };
        })()`, *target.WebSocketDebuggerURL)
		service.mu.Lock()
		service.closeAllCapabilitySessionsLocked()
		service.mu.Unlock()
	}()
	requirements := map[string]CapabilityRequirement{
		UISettingsSectionCapabilityID: {
			Version: "^1.0.0",
			Permissions: json.RawMessage(`{
              "sections":[{
                "id":"compatibility-test",
                "title":"Compatibility Test",
                "group":"personal",
                "icon":"personalization",
                "after":"personalization"
              }]
            }`),
		},
	}
	grants, err := resolvePackageCapabilities("compatibility-test", requirements)
	if err != nil {
		t.Fatal(err)
	}
	payload := Payload{Version: "live-settings-test", Packages: []CompiledPackage{{
		ID: "compatibility-test", Name: "compatibility-test", Version: "1.0.0",
		Capabilities: grants,
		JavaScript: `module.exports.activate = ({ capabilities }) => {
          const settings = capabilities.require("ui.settings-section");
          globalThis.__CODEX_TWEAKS_UI_LIVE_TEST__ = settings.register({
            id: "compatibility-test",
            mount(container) {
              container.textContent = "Codex Tweaks live settings route";
              return () => container.replaceChildren();
            }
          });
        };`,
	}}}
	bridgeID, tokens, configuration, err := service.capabilityBridgeForTargetLocked(ctx, *target, payload)
	if err != nil || configuration == nil {
		t.Fatalf("settings bridge unavailable: %v %#v", err, configuration)
	}
	appModuleURL = configuration.AppModuleURL
	if _, err := service.evaluate(ctx, `(() => {
      const records = [];
      const originalConsoleError = console.error;
      console.error = (...args) => {
        records.push(args.map((value) => {
          try { return value instanceof Error ? value.stack : String(value); }
          catch (_) { return "<unprintable>"; }
        }).join("\n").slice(0, 8000));
        return originalConsoleError.apply(console, args);
      };
      globalThis.__CODEX_TWEAKS_UI_LIVE_ERRORS__ = { originalConsoleError, records };
      return { installed: true };
    })()`, *target.WebSocketDebuggerURL); err != nil {
		t.Fatal(err)
	}
	result, err := service.evaluate(
		ctx,
		injectionScriptWithCapabilities(payload, 0, bridgeID, tokens, configuration),
		*target.WebSocketDebuggerURL,
	)
	if err != nil {
		t.Fatal(err)
	}
	if packageErrors, _ := result["packageErrors"].([]any); len(packageErrors) != 0 {
		t.Fatalf("live package errors: %#v (adapter: %#v)", packageErrors, result["settingsAdapterError"])
	}
	diagnosticsExpression := fmt.Sprintf(`(async () => {
      const slug = globalThis.__CODEX_TWEAKS_UI_LIVE_TEST__?.slug;
      const appModule = await import(%s);
      const visibilityModule = await import(%s);
      const registry = Object.values(appModule).find((value) =>
        Array.isArray(value)
        && value.some((entry) => entry?.slug === "general-settings")
        && value.some((entry) => entry?.slug === "personalization")
      );
      const iconMap = Object.values(visibilityModule).find((value) =>
        value && typeof value === "object"
        && typeof value.personalization === "function"
        && typeof value["general-settings"] === "function"
      );
      const fibers = [];
      for (const element of document.querySelectorAll("*")) {
        for (const key of Object.getOwnPropertyNames(element)) {
          if (key.startsWith("__reactContainer$") || key.startsWith("__reactFiber$")) {
            fibers.push(element[key]?.current ?? element[key]);
          }
        }
      }
      const seenFibers = new Set();
      let routeRegistered = false;
      const flatten = (node, result = [], seen = new Set()) => {
        if (Array.isArray(node)) {
          for (const child of node) flatten(child, result, seen);
        } else if (node && typeof node === "object" && !seen.has(node)) {
          seen.add(node);
          if (node.props) {
            result.push(node);
            flatten(node.props.children, result, seen);
          }
        }
        return result;
      };
      while (fibers.length && seenFibers.size < 200000) {
        const fiber = fibers.shift();
        if (!fiber || typeof fiber !== "object" || seenFibers.has(fiber)) continue;
        seenFibers.add(fiber);
        fibers.push(fiber.child, fiber.sibling, fiber.return, fiber.alternate);
        for (const props of [fiber.memoizedProps, fiber.pendingProps]) {
          const elements = flatten(props?.children);
          if (
            elements.some((element) => element?.props?.path === "/settings")
            && elements.some((element) => element?.props?.path === slug)
          ) routeRegistered = true;
        }
      }
      return {
        slug,
        customIconType: typeof iconMap?.[slug],
        registryHasSlug: registry?.some((entry) => entry?.slug === slug),
        registryOwnFilter: Object.prototype.hasOwnProperty.call(registry ?? {}, "filter"),
        routeRegistered,
        visibleWhenRejected: registry?.filter(() => false).some((entry) => entry?.slug === slug)
      };
    })()`, JSONLiteral(configuration.AppModuleURL), JSONLiteral(configuration.VisibilityModuleURL))
	diagnostics, diagnosticError := service.evaluate(ctx, diagnosticsExpression, *target.WebSocketDebuggerURL)
	if diagnosticError != nil {
		t.Fatal(diagnosticError)
	}
	if diagnostics["registryHasSlug"] != true || diagnostics["registryOwnFilter"] != true ||
		diagnostics["routeRegistered"] != true || diagnostics["visibleWhenRejected"] != true ||
		diagnostics["customIconType"] != "function" {
		t.Fatalf("settings adapter did not mutate the native registries: %#v", diagnostics)
	}
	if _, err := service.evaluate(ctx, `(() => {
      globalThis.__CODEX_TWEAKS_UI_LIVE_TEST__.open();
      return { opened: true };
    })()`, *target.WebSocketDebuggerURL); err != nil {
		t.Fatal(err)
	}
	openedCustom = true
	var observed map[string]any
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		observed, err = service.evaluate(ctx, `(() => {
          const slug = globalThis.__CODEX_TWEAKS_UI_LIVE_TEST__?.slug;
          const buttons = [...document.querySelectorAll("button[data-settings-panel-slug]")];
          const button = buttons.find((candidate) => candidate.dataset.settingsPanelSlug === slug);
          const personalization = buttons.findIndex(
            (candidate) => candidate.dataset.settingsPanelSlug === "personalization"
          );
          return {
            slug,
            path: location.pathname,
            buttonLabel: button?.textContent?.trim() ?? "",
            buttonAria: button?.getAttribute("aria-label") ?? "",
            buttonIndex: buttons.indexOf(button),
            personalizationIndex: personalization,
            content: document.querySelector(
              "[data-codex-tweaks-settings-section-host]"
            )?.textContent ?? "",
            bodyText: document.body?.innerText?.slice(0, 200) ?? "",
            errors: globalThis.__CODEX_TWEAKS_UI_LIVE_ERRORS__?.records ?? []
          };
		})()`, *target.WebSocketDebuggerURL)
		if err == nil && observed["content"] == "Codex Tweaks live settings route" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if observed["buttonLabel"] != "Compatibility Test" || observed["buttonAria"] != "Compatibility Test" ||
		observed["content"] != "Codex Tweaks live settings route" {
		t.Fatalf("live settings section was not native and routable: %#v", observed)
	}
	buttonIndex, _ := observed["buttonIndex"].(float64)
	personalizationIndex, _ := observed["personalizationIndex"].(float64)
	if buttonIndex != personalizationIndex+1 {
		t.Fatalf("live settings placement is wrong: %#v", observed)
	}
	if _, err := service.evaluate(ctx, CleanupScript, *target.WebSocketDebuggerURL); err != nil {
		t.Fatal(err)
	}
	var cleaned map[string]any
	cleanupDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(cleanupDeadline) {
		cleaned, err = service.evaluate(ctx, `(() => {
          const slug = globalThis.__CODEX_TWEAKS_UI_LIVE_TEST__?.slug;
          return {
            adapterPresent: Boolean(globalThis.__CODEX_TWEAKS_SETTINGS_SECTIONS__),
            buttonPresent: [...document.querySelectorAll("button[data-settings-panel-slug]")]
              .some((button) => button.dataset.settingsPanelSlug === slug),
            hostPresent: [...document.querySelectorAll("[data-codex-tweaks-settings-section-host]")]
              .some((host) => host.getAttribute("data-codex-tweaks-settings-section-host") === slug),
            labelHookPresent: Object.prototype.hasOwnProperty.call(Object.prototype, slug)
          };
        })()`, *target.WebSocketDebuggerURL)
		if err == nil && cleaned["adapterPresent"] == false && cleaned["buttonPresent"] == false &&
			cleaned["hostPresent"] == false && cleaned["labelHookPresent"] == false {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if cleaned["adapterPresent"] != false || cleaned["buttonPresent"] != false ||
		cleaned["hostPresent"] != false || cleaned["labelHookPresent"] != false {
		t.Fatalf("settings adapter did not restore the native registries: %#v", cleaned)
	}
	restoreNavigation(ctx)
}

func TestRandomBackgroundPackageLiveRuntime(t *testing.T) {
	if os.Getenv("CODEX_TWEAKS_LIVE_RANDOM_BACKGROUND") != "1" {
		t.Skip("set CODEX_TWEAKS_LIVE_RANDOM_BACKGROUND=1 to inspect the running migrated package")
	}
	service := NewCDPService(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targets, err := service.discoverTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var target *CDPTarget
	for index := range targets {
		if !strings.Contains(targets[index].URL, "initialRoute=") {
			target = &targets[index]
			break
		}
	}
	if target == nil {
		t.Fatal("no main Codex target")
	}
	permissions, err := normalizeUISettingsSectionPermissions(json.RawMessage(
		`{"sections":[{"id":"random-background","title":"随机背景"}]}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	bound, err := bindUISettingsSectionPermissions("codex-random-background", permissions)
	if err != nil {
		t.Fatal(err)
	}
	settings := UISettingsSectionPermissions{}
	if err := json.Unmarshal(bound, &settings); err != nil {
		t.Fatal(err)
	}
	slug := settings.Sections[0].Slug
	expression := fmt.Sprintf(`(() => {
      const runtime = globalThis.__CODEX_TWEAKS__;
      const adapter = globalThis.__CODEX_TWEAKS_SETTINGS_SECTIONS__;
      const packageError = runtime?.packageErrors?.find(
        (entry) => entry?.id === "codex-random-background"
      );
      return {
        runtimePresent: Boolean(runtime),
        runtimeVersion: runtime?.version ?? "",
        packageError: packageError?.message ?? "",
        settingsRegistered: adapter?.has?.(%s) === true,
        activeSettingsSlug: document.querySelector(
          'button[data-settings-panel-slug][aria-current="page"]'
        )?.dataset?.settingsPanelSlug ?? "",
        settingsButtonSlugs: [...document.querySelectorAll(
          'button[data-settings-panel-slug]'
        )].map((button) => button.dataset.settingsPanelSlug),
        panelHostSlug: document.querySelector(
          '[data-codex-tweaks-rbgp-settings-panel]'
        )?.closest('[data-codex-tweaks-settings-section-host]')
          ?.getAttribute('data-codex-tweaks-settings-section-host') ?? "",
        packageRootPresent: Boolean(document.querySelector(
          '[data-codex-tweaks-package-root="codex-random-background"]'
        )),
        packageRootCount: document.querySelectorAll(
          '[data-codex-tweaks-package-root="codex-random-background"]'
        ).length,
        packageStylePresent: Boolean(document.querySelector(
          'style[data-codex-tweaks-package-style="codex-random-background"]'
        )),
        packageStyleCount: document.querySelectorAll(
          'style[data-codex-tweaks-package-style="codex-random-background"]'
        ).length,
        legacyEmbeds: [...document.querySelectorAll(
          '[data-codex-tweaks-rbgp-embed]'
        )].map((element) => ({
          parentTag: element.parentElement?.tagName ?? "",
          parentClass: element.parentElement?.className ?? "",
          text: element.textContent?.trim()?.slice(0, 80) ?? ""
        }))
      };
    })()`, JSONLiteral(slug))
	observed, err := service.evaluate(ctx, expression, *target.WebSocketDebuggerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("running random-background diagnostics: %#v", observed)
	if observed["runtimePresent"] != true || observed["packageError"] != "" ||
		observed["settingsRegistered"] != true || observed["packageRootPresent"] != true ||
		observed["packageStylePresent"] != true {
		t.Fatalf("migrated package is not fully active: %#v", observed)
	}
}

func TestRandomBackgroundSettingsRouteLive(t *testing.T) {
	if os.Getenv("CODEX_TWEAKS_LIVE_RANDOM_BACKGROUND_ROUTE") != "1" {
		t.Skip("set CODEX_TWEAKS_LIVE_RANDOM_BACKGROUND_ROUTE=1 to verify settings route ownership")
	}
	service := NewCDPService(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	targets, err := service.discoverTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var target *CDPTarget
	for index := range targets {
		if !strings.Contains(targets[index].URL, "initialRoute=") {
			target = &targets[index]
			break
		}
	}
	if target == nil {
		t.Fatal("no main Codex target")
	}
	permissions, err := normalizeUISettingsSectionPermissions(json.RawMessage(`{
      "sections":[{
        "id":"random-background",
        "title":"随机背景",
        "group":"personal",
        "icon":"personalization",
        "after":"personalization"
      }]
    }`))
	if err != nil {
		t.Fatal(err)
	}
	bound, err := bindUISettingsSectionPermissions("codex-random-background", permissions)
	if err != nil {
		t.Fatal(err)
	}
	settings := UISettingsSectionPermissions{}
	if err := json.Unmarshal(bound, &settings); err != nil {
		t.Fatal(err)
	}
	slug := settings.Sections[0].Slug
	payload := Payload{Packages: []CompiledPackage{{
		ID: "codex-random-background",
		Capabilities: map[string]GrantedCapability{
			UISettingsSectionCapabilityID: {
				Version: UISettingsSectionCapabilityVersion, Permissions: bound,
			},
		},
	}}}
	session, err := openCapabilitySession(
		ctx, service.dialer, service.AllowedOrigin, *target, service.broker, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := session.ensureSettingsAdapter(ctx, payload)
	defer session.Close()
	if err != nil || configuration == nil {
		t.Fatalf("settings module unavailable: %v %#v", err, configuration)
	}

	observe := func() map[string]any {
		observed, evaluateError := service.evaluate(ctx, `(() => {
          const panel = document.querySelector('[data-codex-tweaks-rbgp-settings-panel]');
          return {
            activeSlug: document.querySelector(
              'button[data-settings-panel-slug][aria-current="page"]'
            )?.dataset?.settingsPanelSlug ?? "",
            buttonSlugs: [...document.querySelectorAll(
              'button[data-settings-panel-slug]'
            )].map((button) => button.dataset.settingsPanelSlug),
            panelCount: document.querySelectorAll(
              '[data-codex-tweaks-rbgp-settings-panel]'
            ).length,
            legacyEmbedCount: document.querySelectorAll(
              '[data-codex-tweaks-rbgp-embed]'
            ).length,
            legacyEmbedDetails: [...document.querySelectorAll(
              '[data-codex-tweaks-rbgp-embed]'
            )].map((element) => ({
              parentTag: element.parentElement?.tagName ?? "",
              parentClass: element.parentElement?.className ?? "",
              text: element.textContent?.trim()?.slice(0, 80) ?? ""
            })),
            panelHostSlug: panel?.closest(
              '[data-codex-tweaks-settings-section-host]'
            )?.getAttribute('data-codex-tweaks-settings-section-host') ?? "",
            panelText: panel?.textContent?.trim()?.slice(0, 120) ?? ""
          };
        })()`, *target.WebSocketDebuggerURL)
		if evaluateError != nil {
			t.Fatal(evaluateError)
		}
		return observed
	}
	original := observe()
	originalSlug, _ := original["activeSlug"].(string)
	navigate := func(sectionSlug string, replace bool) {
		expression := fmt.Sprintf(`(async () => {
          const appModule = await import(%s);
          const bus = Object.values(appModule).find((value) =>
            value && typeof value === "object"
            && value.handlers instanceof Map
            && typeof value.dispatchHostMessage === "function"
          );
          if (!bus) throw new Error("Codex navigation bus unavailable");
          bus.dispatchHostMessage({
            type: "navigate-to-route",
            path: "/settings/" + %s,
            replace: %s
          });
          return { navigated: true };
        })()`, JSONLiteral(configuration.AppModuleURL), JSONLiteral(sectionSlug), JSONLiteral(replace))
		if _, evaluateError := service.evaluate(ctx, expression, *target.WebSocketDebuggerURL); evaluateError != nil {
			t.Fatal(evaluateError)
		}
	}
	restore := func() {
		if originalSlug != "" {
			navigate(originalSlug, true)
			return
		}
		expression := fmt.Sprintf(`(async () => {
          const appModule = await import(%s);
          const bus = Object.values(appModule).find((value) =>
            value && typeof value === "object"
            && value.handlers instanceof Map
            && typeof value.dispatchHostMessage === "function"
          );
          bus?.dispatchHostMessage({ type: "navigate-back" });
          return { restored: Boolean(bus) };
        })()`, JSONLiteral(configuration.AppModuleURL))
		_, _ = service.evaluate(context.Background(), expression, *target.WebSocketDebuggerURL)
	}
	defer restore()

	waitFor := func(expectedSlug string, expectedPanelCount, expectedLegacyEmbedCount float64) map[string]any {
		deadline := time.Now().Add(3 * time.Second)
		var observed map[string]any
		for time.Now().Before(deadline) {
			observed = observe()
			if observed["activeSlug"] == expectedSlug &&
				observed["panelCount"] == expectedPanelCount &&
				observed["legacyEmbedCount"] == expectedLegacyEmbedCount {
				return observed
			}
			time.Sleep(50 * time.Millisecond)
		}
		return observed
	}

	navigate(slug, originalSlug != "")
	randomPage := waitFor(slug, 1, 0)
	if randomPage["panelHostSlug"] != slug {
		t.Fatalf("random-background panel is not owned by its route: %#v", randomPage)
	}
	navigate("personalization", true)
	personalizationPage := waitFor("personalization", 0, 0)
	if personalizationPage["panelCount"] != float64(0) ||
		personalizationPage["legacyEmbedCount"] != float64(0) {
		t.Fatalf("random-background settings leaked into personalization: %#v", personalizationPage)
	}
	t.Logf("random page: %#v", randomPage)
	t.Logf("personalization page: %#v", personalizationPage)
}
