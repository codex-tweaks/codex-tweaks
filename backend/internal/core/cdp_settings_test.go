package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testCompiledSettingsUI(
	t *testing.T,
	packageID string,
	section UISettingsSectionDeclaration,
) (CompiledPackageUI, RuntimeSettingsSection) {
	t.Helper()
	required := false
	configuration := PackageUIConfiguration{SettingsSections: &SettingsSectionsExtension{
		APIVersion: SettingsSectionsAPIVersion,
		Required:   &required,
		Items:      []UISettingsSectionDeclaration{section},
	}}
	if err := normalizePackageUI(&configuration); err != nil {
		t.Fatal(err)
	}
	compiled, err := compilePackageUI(packageID, configuration)
	if err != nil {
		t.Fatal(err)
	}
	return compiled, compiled.SettingsSections.Items[0]
}

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

func TestSettingsAdapterDiscoveryIsCachedUntilExecutionContextClears(t *testing.T) {
	const (
		appModuleURL        = "app://-/assets/app-initial-TEST.js"
		settingsModuleURL   = "app://-/assets/settings-page-TEST.js"
		visibilityModuleURL = "app://-/assets/use-visible-settings-sections-TEST.js"
		navigationModuleURL = "app://-/assets/message-bus-TEST.js"
	)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	var countsMu sync.Mutex
	getScriptSourceCount := 0
	failSource := false
	importModuleCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		writeResponse := func(command map[string]any, result map[string]any) {
			_ = connection.WriteJSON(map[string]any{"id": command["id"], "result": result})
		}
		writeScriptParsed := func(scriptID, scriptURL string) {
			_ = connection.WriteJSON(map[string]any{
				"method": "Debugger.scriptParsed",
				"params": map[string]any{"scriptId": scriptID, "url": scriptURL},
			})
		}
		for {
			command := map[string]any{}
			if connection.ReadJSON(&command) != nil {
				return
			}
			method, _ := command["method"].(string)
			switch method {
			case "Runtime.enable", "Runtime.addBinding":
				writeResponse(command, map[string]any{})
			case "Debugger.enable":
				writeScriptParsed("app-1", appModuleURL)
				writeResponse(command, map[string]any{})
			case "Debugger.getScriptSource":
				params, _ := command["params"].(map[string]any)
				scriptID, _ := params["scriptId"].(string)
				countsMu.Lock()
				getScriptSourceCount++
				shouldFail := failSource
				countsMu.Unlock()
				if shouldFail {
					writeResponse(command, map[string]any{"scriptSource": "const missing = true;"})
					continue
				}
				source := `import "./use-visible-settings-sections-TEST.js";`
				if strings.HasPrefix(scriptID, "app-") {
					source = `const deps=["./settings-page-TEST.js"];import{bus}from"./message-bus-TEST.js";`
				}
				writeResponse(command, map[string]any{"scriptSource": source})
			case "Runtime.getProperties":
				params, _ := command["params"].(map[string]any)
				switch params["objectId"] {
				case "exports":
					writeResponse(command, map[string]any{"result": []any{map[string]any{"name": "0", "value": map[string]any{"type": "function", "objectId": "accessor"}}}})
				case "accessor":
					writeResponse(command, map[string]any{"internalProperties": []any{map[string]any{"name": "[[Scopes]]", "value": map[string]any{"objectId": "scopes"}}}})
				case "scopes":
					writeResponse(command, map[string]any{"result": []any{
						map[string]any{"name": "0", "value": map[string]any{"description": "Module", "objectId": "module"}},
						map[string]any{"name": "1", "value": map[string]any{"description": "Global", "objectId": "forbidden"}},
					}})
				case "module":
					writeResponse(command, map[string]any{"result": []any{map[string]any{"name": "privateIcons", "value": map[string]any{"type": "object", "objectId": "icons"}}}})
				default:
					t.Errorf("unexpected scope read: %v", params["objectId"])
					writeResponse(command, map[string]any{})
				}
			case "Runtime.callFunctionOn":
				writeResponse(command, map[string]any{"result": map[string]any{"value": true}})
			case "Runtime.releaseObjectGroup":
				writeResponse(command, map[string]any{})
			case "Runtime.evaluate":
				params, _ := command["params"].(map[string]any)
				expression, _ := params["expression"].(string)
				if strings.Contains(expression, "dispatchHostMessage") {
					writeResponse(command, map[string]any{"result": map[string]any{"value": navigationModuleURL}})
					continue
				}
				if strings.Contains(expression, "Object.values(module)") {
					writeResponse(command, map[string]any{"result": map[string]any{"objectId": "exports"}})
					continue
				}
				countsMu.Lock()
				importModuleCount++
				settingsScriptID := fmt.Sprintf("settings-%d", importModuleCount)
				countsMu.Unlock()
				writeScriptParsed(settingsScriptID, settingsModuleURL)
				writeResponse(command, map[string]any{"result": map[string]any{"value": true}})
			case "Test.resetExecutionContext":
				_ = connection.WriteJSON(map[string]any{
					"method": "Runtime.executionContextsCleared", "params": map[string]any{},
				})
				writeScriptParsed("app-2", appModuleURL)
				writeResponse(command, map[string]any{})
			default:
				_ = connection.WriteJSON(map[string]any{
					"id": command["id"], "error": map[string]any{"message": "unexpected method " + method},
				})
			}
		}
	}))
	defer server.Close()

	debuggerURL := "ws" + strings.TrimPrefix(server.URL, "http")
	target := CDPTarget{
		ID: "codex-main", Type: "page", URL: "app://-/index.html", WebSocketDebuggerURL: &debuggerURL,
	}
	service := NewCDPService(nil)
	session, err := openRendererBridgeSession(
		context.Background(), service.dialer, server.URL, target, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	makePayload := func(title string) Payload {
		return Payload{Packages: []CompiledPackage{{
			ID: "sample",
			UI: CompiledPackageUI{SettingsSections: &CompiledSettingsSections{Items: []RuntimeSettingsSection{{
				PackageID: "sample", ID: "wallpaper", Title: title, Slug: "sample-wallpaper",
			}}}},
		}}}
	}
	first, err := session.ensureSettingsAdapter(context.Background(), makePayload("First"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.ensureSettingsAdapter(context.Background(), makePayload("Second"))
	if err != nil {
		t.Fatal(err)
	}
	if first.AppModuleURL != appModuleURL || first.VisibilityModuleURL != visibilityModuleURL ||
		first.NavigationModuleURL != navigationModuleURL || first.IconRegistryKey != settingsIconRegistryKey ||
		second.AppModuleURL != appModuleURL || second.VisibilityModuleURL != visibilityModuleURL ||
		len(second.Sections) != 1 || second.Sections[0].Title != "Second" {
		t.Fatalf("unexpected cached settings adapters: first=%#v second=%#v", first, second)
	}
	countsMu.Lock()
	if getScriptSourceCount != 2 || importModuleCount != 1 {
		t.Fatalf("cached discovery repeated CDP work: getScriptSource=%d import=%d", getScriptSourceCount, importModuleCount)
	}
	countsMu.Unlock()

	if _, err := session.call(context.Background(), "Test.resetExecutionContext", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	third, err := session.ensureSettingsAdapter(context.Background(), makePayload("Third"))
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Sections) != 1 || third.Sections[0].Title != "Third" {
		t.Fatalf("settings sections were not refreshed after navigation: %#v", third)
	}
	countsMu.Lock()
	if getScriptSourceCount != 4 || importModuleCount != 2 {
		t.Fatalf("execution-context reset did not invalidate discovery: getScriptSource=%d import=%d", getScriptSourceCount, importModuleCount)
	}
	failSource = true
	countsMu.Unlock()
	if _, err := session.call(context.Background(), "Test.resetExecutionContext", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := session.ensureSettingsAdapter(context.Background(), makePayload("Failure")); err == nil {
			t.Fatal("missing module accepted")
		}
	}
	countsMu.Lock()
	if getScriptSourceCount != 5 {
		t.Fatalf("failed discovery was repeated: %d", getScriptSourceCount)
	}
	failSource = false
	countsMu.Unlock()
	if _, err := session.call(context.Background(), "Test.resetExecutionContext", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ensureSettingsAdapter(context.Background(), makePayload("Recovered")); err != nil {
		t.Fatal(err)
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
	ui, _ := testCompiledSettingsUI(t, "compatibility-test", UISettingsSectionDeclaration{
		ID: "compatibility-test", Title: "Compatibility Test",
	})
	payload := Payload{Packages: []CompiledPackage{{
		ID: "compatibility-test",
		UI: ui,
	}}}
	errorsByTarget := []string{}
	for _, target := range targets {
		if strings.Contains(target.URL, "initialRoute=") {
			continue
		}
		session, openError := openRendererBridgeSession(
			ctx, service.dialer, service.AllowedOrigin, target, nil, nil,
		)
		if openError != nil {
			errorsByTarget = append(errorsByTarget, target.ID+": "+openError.Error())
			continue
		}
		configuration, adapterError := session.ensureSettingsAdapter(ctx, payload)
		session.Close()
		if adapterError == nil && configuration != nil {
			t.Logf("Codex settings navigation module: %s", configuration.NavigationModuleURL)
			return
		}
		if adapterError != nil {
			errorsByTarget = append(errorsByTarget, target.ID+": "+adapterError.Error())
		}
	}
	t.Fatalf("no live Codex target accepted ui.settingsSections@1: %s", strings.Join(errorsByTarget, "; "))
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
	navigationModuleURL := ""
	openedCustom := false
	restoreNavigation := func(restoreContext context.Context) {
		if !openedCustom || appModuleURL == "" {
			return
		}
		restoreExpression := fmt.Sprintf(`(async () => {
          const appModule = await import(%s);
          const bus = Object.values(appModule).find((value) =>
            value && typeof value === "object"
            && typeof value.dispatchHostMessage === "function"
          );
          if (!bus) return { restored: false };
          %s
          return { restored: true };
        })()`, JSONLiteral(navigationModuleURL), func() string {
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
		service.closeAllRendererSessionsLocked()
		service.mu.Unlock()
	}()
	ui, _ := testCompiledSettingsUI(t, "compatibility-test", UISettingsSectionDeclaration{
		ID: "compatibility-test", Title: "Compatibility Test", Group: "personal",
		Icon: "personalization", After: "personalization",
	})
	payload := Payload{Version: "live-settings-test", Packages: []CompiledPackage{{
		ID: "compatibility-test", Name: "compatibility-test", Version: "1.0.0",
		UI: ui,
		JavaScript: `module.exports.activate = ({ ui }) => {
          const settings = ui.settingsSections;
          globalThis.__CODEX_TWEAKS_UI_LIVE_TEST__ = settings.register({
            id: "compatibility-test",
            mount(container) {
              container.textContent = "Codex Tweaks live settings route";
              return () => container.replaceChildren();
            }
          });
        };`,
	}}}
	bridgeID, tokens, configuration, err := service.rendererBridgeForTargetLocked(ctx, *target, payload)
	if err != nil || configuration == nil {
		t.Fatalf("settings bridge unavailable: %v %#v", err, configuration)
	}
	appModuleURL = configuration.AppModuleURL
	navigationModuleURL = configuration.NavigationModuleURL
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
		injectionScriptWithRendererBridge(payload, 0, bridgeID, tokens, configuration),
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
      const iconMap = globalThis.__CODEX_TWEAKS_SETTINGS_ICON_REGISTRY__ ?? Object.values(visibilityModule).find((value) =>
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
        customIconValid: typeof iconMap?.[slug] === "function" || Boolean(iconMap?.[slug]?.component),
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
		diagnostics["customIconValid"] != true {
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

	// An unavailable optional adapter must not unload a working package on each monitor tick.
	failedConfiguration := *configuration
	failedConfiguration.VisibilityModuleURL = "data:text/javascript,export{}"
	failedConfiguration.IconRegistryKey = ""
	fallbackPayload := Payload{Version: "live-settings-failure", Packages: []CompiledPackage{{
		ID: "fallback", Name: "fallback", UI: ui,
		JavaScript: `module.exports.activate = ({ root }) => { root.textContent = "background survived"; };`,
	}}}
	injected, err := service.evaluate(ctx, injectionScriptWithRendererBridge(fallbackPayload, 0, bridgeID, tokens, &failedConfiguration), *target.WebSocketDebuggerURL)
	if err != nil || injected["settingsAdapterError"] == nil {
		t.Fatalf("adapter failure was not observed: %v %#v", err, injected)
	}
	for range 3 {
		for _, script := range []string{
			injectionRuntimeProbeScript(fallbackPayload, 0, bridgeID, &failedConfiguration),
			injectionScriptWithRendererBridge(fallbackPayload, 0, bridgeID, tokens, &failedConfiguration),
		} {
			stable, err := service.evaluate(ctx, script, *target.WebSocketDebuggerURL)
			if err != nil || stable["status"] != "unchanged" || stable["settingsAdapterError"] == nil {
				t.Fatalf("failed adapter retriggered injection: %v %#v", err, stable)
			}
		}
	}
	survivor, err := service.evaluate(ctx, `({content: document.querySelector('[data-codex-tweaks-package-root="fallback"]')?.textContent})`, *target.WebSocketDebuggerURL)
	if err != nil || survivor["content"] != "background survived" {
		t.Fatalf("optional adapter failure removed package: %v %#v", err, survivor)
	}
}

func TestCustomBackgroundPackageLiveRuntime(t *testing.T) {
	if os.Getenv("CODEX_TWEAKS_LIVE_CUSTOM_BACKGROUND") != "1" {
		t.Skip("set CODEX_TWEAKS_LIVE_CUSTOM_BACKGROUND=1 to inspect the running migrated package")
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
	_, section := testCompiledSettingsUI(t, "codex-custom-background", UISettingsSectionDeclaration{
		ID: "custom-background", Title: "自定义背景",
	})
	slug := section.Slug
	expression := fmt.Sprintf(`(() => {
      const runtime = globalThis.__CODEX_TWEAKS__;
      const adapter = globalThis.__CODEX_TWEAKS_SETTINGS_SECTIONS__;
      const packageError = runtime?.packageErrors?.find(
        (entry) => entry?.id === "codex-custom-background"
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
          '[data-codex-tweaks-cbgp-settings-panel]'
        )?.closest('[data-codex-tweaks-settings-section-host]')
          ?.getAttribute('data-codex-tweaks-settings-section-host') ?? "",
        packageRootPresent: Boolean(document.querySelector(
          '[data-codex-tweaks-package-root="codex-custom-background"]'
        )),
        packageRootCount: document.querySelectorAll(
          '[data-codex-tweaks-package-root="codex-custom-background"]'
        ).length,
        packageStylePresent: Boolean(document.querySelector(
          'style[data-codex-tweaks-package-style="codex-custom-background"]'
        )),
        packageStyleCount: document.querySelectorAll(
          'style[data-codex-tweaks-package-style="codex-custom-background"]'
        ).length,
        legacyEmbeds: [...document.querySelectorAll(
          '[data-codex-tweaks-cbgp-embed]'
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
	t.Logf("running custom-background diagnostics: %#v", observed)
	if observed["runtimePresent"] != true || observed["packageError"] != "" ||
		observed["settingsRegistered"] != true || observed["packageRootPresent"] != true ||
		observed["packageStylePresent"] != true {
		t.Fatalf("migrated package is not fully active: %#v", observed)
	}
}

func TestCustomBackgroundSettingsRouteLive(t *testing.T) {
	if os.Getenv("CODEX_TWEAKS_LIVE_CUSTOM_BACKGROUND_ROUTE") != "1" {
		t.Skip("set CODEX_TWEAKS_LIVE_CUSTOM_BACKGROUND_ROUTE=1 to verify settings route ownership")
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
	ui, section := testCompiledSettingsUI(t, "ct-custom-background", UISettingsSectionDeclaration{
		ID: "custom-background", Title: "自定义背景", Group: "personal",
		Icon: "personalization", After: "personalization",
	})
	slug := section.Slug
	payload := Payload{Packages: []CompiledPackage{{
		ID: "ct-custom-background",
		UI: ui,
	}}}
	session, err := openRendererBridgeSession(
		ctx, service.dialer, service.AllowedOrigin, *target, nil, nil,
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
          const panel = document.querySelector('[data-codex-tweaks-cbgp-settings-panel]');
          return {
            activeSlug: document.querySelector(
              'button[data-settings-panel-slug][aria-current="page"]'
            )?.dataset?.settingsPanelSlug ?? "",
            buttonSlugs: [...document.querySelectorAll(
              'button[data-settings-panel-slug]'
            )].map((button) => button.dataset.settingsPanelSlug),
            panelCount: document.querySelectorAll(
              '[data-codex-tweaks-cbgp-settings-panel]'
            ).length,
            legacyEmbedCount: document.querySelectorAll(
              '[data-codex-tweaks-cbgp-embed]'
            ).length,
            legacyEmbedDetails: [...document.querySelectorAll(
              '[data-codex-tweaks-cbgp-embed]'
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
        })()`, JSONLiteral(configuration.NavigationModuleURL), JSONLiteral(sectionSlug), JSONLiteral(replace))
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
        })()`, JSONLiteral(configuration.NavigationModuleURL))
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
	customPage := waitFor(slug, 1, 0)
	if customPage["panelHostSlug"] != slug {
		t.Fatalf("custom-background panel is not owned by its route: %#v", customPage)
	}
	navigate("personalization", true)
	personalizationPage := waitFor("personalization", 0, 0)
	if personalizationPage["panelCount"] != float64(0) ||
		personalizationPage["legacyEmbedCount"] != float64(0) {
		t.Fatalf("custom-background settings leaked into personalization: %#v", personalizationPage)
	}
	t.Logf("custom page: %#v", customPage)
	t.Logf("personalization page: %#v", personalizationPage)
}
