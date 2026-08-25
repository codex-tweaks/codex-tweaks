package core

import (
	"fmt"
	"strings"
)

const CleanupScript = `(() => {
  const key = "__CODEX_TWEAKS__";
  try { globalThis[key]?.cleanup?.(); } catch (_) {}
  document.getElementById("codex-tweaks-root")?.remove();
  delete globalThis[key];
  return { status: "cleaned" };
})()`

func InjectionScript(payload Payload, forceGeneration int) string {
	return injectionScriptWithCapabilities(payload, forceGeneration, "", map[string]string{}, nil)
}

func injectionScriptWithCapabilities(
	payload Payload,
	forceGeneration int,
	bridgeSessionID string,
	capabilityTokens map[string]string,
	settingsAdapterConfiguration *SettingsAdapterConfiguration,
) string {
	effectiveVersion := payload.Version
	if forceGeneration != 0 {
		effectiveVersion = fmt.Sprintf("%s-force-%d", payload.Version, forceGeneration)
	}
	setups := make([]string, 0, len(payload.Packages))
	executions := make([]string, 0, len(payload.Packages))
	for _, pkg := range payload.Packages {
		setups = append(setups, packageSetupScript(pkg))
		executions = append(executions, packageExecutionScript(pkg, capabilityTokens[pkg.ID]))
	}
	return fmt.Sprintf(`(async () => {
  const key = "__CODEX_TWEAKS__";
  const version = %s;
  const bridgeSessionID = %s;
  const settingsAdapterConfiguration = %s;
  const settingsAdapterKey = JSON.stringify(settingsAdapterConfiguration ?? null);
  const settingsAdapterExpected = Boolean(settingsAdapterConfiguration?.sections?.length);
  const existing = globalThis[key];
  if (
    existing?.version === version &&
    existing?.bridgeSessionID === bridgeSessionID &&
    existing?.settingsAdapterKey === settingsAdapterKey &&
    (!settingsAdapterExpected || existing?.settingsAdapterReady === true) &&
    document.getElementById("codex-tweaks-root")
  ) {
    return {
      status: "unchanged",
      version,
      packageErrors: existing.packageErrors ?? []
    };
  }

  try { existing?.cleanup?.(); } catch (_) {}
  document.getElementById("codex-tweaks-root")?.remove();

  const host = document.createElement("div");
  host.id = "codex-tweaks-root";
  host.style.display = "contents";
  (document.body || document.documentElement).appendChild(host);
  const packageStates = new Map();
  const packageErrors = [];
  const capabilityPending = new Map();
  const capabilityPendingLimit = 64;
  const capabilityBinding = bridgeSessionID
    ? globalThis[%s]
    : null;
  let capabilitySequence = 0;

  const capabilityError = (message, code) => {
    const error = new Error(message);
    if (code) error.code = code;
    return error;
  };

  const invokeCapability = (token, capability, method, parameters) => {
    if (!bridgeSessionID || typeof capabilityBinding !== "function") {
      return Promise.reject(capabilityError(
        "Codex Tweaks host capability bridge is unavailable",
        "bridge_unavailable"
      ));
    }
    if (capabilityPending.size >= capabilityPendingLimit) {
      return Promise.reject(capabilityError(
        "Capability bridge is busy",
        "busy"
      ));
    }
    const randomPart = globalThis.crypto?.randomUUID?.()
      ?? (Date.now().toString(36) + "-" + Math.random().toString(36).slice(2));
    const id = bridgeSessionID + ":" + (++capabilitySequence) + ":" + randomPart;
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        capabilityPending.delete(id);
        reject(capabilityError("Capability request timed out", "timeout"));
      }, 20000);
      capabilityPending.set(id, { resolve, reject, timeout });
      try {
        capabilityBinding(JSON.stringify({
          v: 1,
          id,
          token,
          capability,
          method,
          parameters: parameters ?? {}
        }));
      } catch (error) {
        clearTimeout(timeout);
        capabilityPending.delete(id);
        reject(error);
      }
    });
  };

  let settingsAdapter = null;
  let settingsAdapterError = null;
  const installSettingsAdapter = async (configuration) => {
    if (!configuration?.sections?.length) return null;
    const globalKey = "__CODEX_TWEAKS_SETTINGS_SECTIONS__";
    try { globalThis[globalKey]?.cleanup?.(); } catch (_) {}
    const descriptors = new Map(
      configuration.sections.map((section) => [section.slug, Object.freeze({ ...section })])
    );
    const renderers = new Map();
    const mounted = new Map();
    const routeElements = new Map();
    const previousIcons = new Map();
    const labelMessages = new Map();
    const previousLabels = new Map();
    const prototypeLabelHooks = new Map();
    let registry = null;
    let registryFilter = null;
    let originalRegistryFilterDescriptor = null;
    let iconMap = null;
    let labelRegistry = null;
    let groupRegistry = null;
    let navigationBus = null;
    let settingsRouteChildren = null;
    let routeTemplate = null;

    const originalArrayMapDescriptor = Object.getOwnPropertyDescriptor(Array.prototype, "map");
    const originalArrayMap = originalArrayMapDescriptor?.value;
    let groupMapInterceptor = null;

    const restoreGroupMapInterceptor = () => {
      if (groupMapInterceptor && Array.prototype.map === groupMapInterceptor) {
        Object.defineProperty(Array.prototype, "map", originalArrayMapDescriptor);
      }
      groupMapInterceptor = null;
    };
    const isSettingsGroupRegistry = (value) =>
      Array.isArray(value)
      && value.some((group) => group?.key === "personal" && group.slugs?.includes?.("personalization"))
      && value.some((group) => group?.key === "integrations")
      && value.some((group) => group?.key === "coding");
    const applyGroupPlacements = () => {
      if (!groupRegistry) return;
      for (const group of groupRegistry) {
        if (!Array.isArray(group?.slugs)) continue;
        for (let index = group.slugs.length - 1; index >= 0; index -= 1) {
          if (descriptors.has(group.slugs[index])) group.slugs.splice(index, 1);
        }
      }
      for (const descriptor of descriptors.values()) {
        if (!renderers.has(descriptor.slug)) continue;
        const group = groupRegistry.find((candidate) => candidate?.key === descriptor.group)
          ?? groupRegistry.find((candidate) => candidate?.key === "personal")
          ?? groupRegistry[0];
        if (!Array.isArray(group?.slugs)) continue;
        const afterIndex = descriptor.after ? group.slugs.indexOf(descriptor.after) : -1;
        group.slugs.splice(afterIndex >= 0 ? afterIndex + 1 : group.slugs.length, 0, descriptor.slug);
      }
    };
    if (typeof originalArrayMap === "function") {
      groupMapInterceptor = function (callback, thisArg) {
        if (!groupRegistry && isSettingsGroupRegistry(this)) {
          groupRegistry = this;
          restoreGroupMapInterceptor();
          applyGroupPlacements();
        }
        return originalArrayMap.call(this, callback, thisArg);
      };
      Object.defineProperty(Array.prototype, "map", {
        ...originalArrayMapDescriptor,
        value: groupMapInterceptor
      });
    }

    const flattenReactElements = (node, result = [], seen = new Set(), depth = 0) => {
      if (node == null || depth > 24) return result;
      if (Array.isArray(node)) {
        for (const child of node) flattenReactElements(child, result, seen, depth + 1);
        return result;
      }
      if (typeof node !== "object" || seen.has(node)) return result;
      seen.add(node);
      if (node.props) {
        result.push(node);
        flattenReactElements(node.props.children, result, seen, depth + 1);
      }
      return result;
    };
    const findSettingsRouteElement = () => {
      const fibers = [];
      for (const element of document.querySelectorAll("*")) {
        for (const key of Object.getOwnPropertyNames(element)) {
          if (key.startsWith("__reactContainer$") || key.startsWith("__reactFiber$")) {
            fibers.push(element[key]?.current ?? element[key]);
          }
        }
      }
      const seen = new Set();
      while (fibers.length && seen.size < 200000) {
        const fiber = fibers.shift();
        if (!fiber || typeof fiber !== "object" || seen.has(fiber)) continue;
        seen.add(fiber);
        fibers.push(fiber.child, fiber.sibling, fiber.return, fiber.alternate);
        for (const props of [fiber.memoizedProps, fiber.pendingProps]) {
          const route = flattenReactElements(props?.children).find(
            (candidate) =>
              candidate?.props?.path === "/settings"
              && Array.isArray(candidate.props.children)
              && flattenReactElements(candidate.props.children).some(
                (child) => child?.props?.path === "*"
              )
          );
          if (route) return route;
        }
      }
      return null;
    };
    const installRoute = (descriptor) => {
      if (routeElements.has(descriptor.slug)) return;
      const hostElement = {
        ...routeTemplate,
        key: null,
        type: "div",
        props: {
          "aria-label": descriptor.title,
          "data-codex-tweaks-settings-section-host": descriptor.slug,
          className: "h-full min-h-0 overflow-y-auto",
          role: "tabpanel"
        },
        _owner: null
      };
      const routeElement = {
        ...routeTemplate,
        key: "codex-tweaks-settings-" + descriptor.slug,
        props: {
          ...routeTemplate.props,
          children: undefined,
          element: hostElement,
          index: undefined,
          path: descriptor.slug
        },
        _owner: null
      };
      const wildcardIndex = settingsRouteChildren.findIndex(
        (candidate) => candidate?.props?.path === "*"
      );
      settingsRouteChildren.splice(
        wildcardIndex >= 0 ? wildcardIndex : settingsRouteChildren.length,
        0,
        routeElement
      );
      routeElements.set(descriptor.slug, routeElement);
    };
    const removeRoute = (slug) => {
      const routeElement = routeElements.get(slug);
      if (!routeElement || !settingsRouteChildren) return;
      const index = settingsRouteChildren.indexOf(routeElement);
      if (index >= 0) settingsRouteChildren.splice(index, 1);
      routeElements.delete(slug);
    };

    const labelMessage = (descriptor) => Object.freeze({
      id: "codex-tweaks.settings." + descriptor.slug,
      defaultMessage: descriptor.title,
      description: "Settings section supplied by Codex Tweaks package " + descriptor.packageID
    });
    const restorePrototypeLabelHook = (slug) => {
      const hook = prototypeLabelHooks.get(slug);
      if (!hook) return;
      const current = Object.getOwnPropertyDescriptor(Object.prototype, slug);
      if (current?.get === hook.getter) {
        if (hook.previous) Object.defineProperty(Object.prototype, slug, hook.previous);
        else delete Object.prototype[slug];
      }
      prototypeLabelHooks.delete(slug);
    };
    const writePrivateLabel = (slug, message) => {
      if (!labelRegistry) return false;
      if (!previousLabels.has(slug)) {
        previousLabels.set(slug, Object.getOwnPropertyDescriptor(labelRegistry, slug) ?? null);
      }
      try {
        Object.defineProperty(labelRegistry, slug, {
          configurable: true,
          enumerable: true,
          writable: true,
          value: message
        });
        restorePrototypeLabelHook(slug);
        return labelRegistry[slug] === message;
      } catch (_) {
        return false;
      }
    };
    const installLabel = (descriptor) => {
      const slug = descriptor.slug;
      const message = labelMessage(descriptor);
      labelMessages.set(slug, message);
      if (writePrivateLabel(slug, message) || prototypeLabelHooks.has(slug)) return;
      const previous = Object.getOwnPropertyDescriptor(Object.prototype, slug) ?? null;
      const getter = function () {
        const generalMessage = this?.["general-settings"];
        const personalizationMessage = this?.personalization;
        if (
          this && typeof this === "object"
          && Object.prototype.hasOwnProperty.call(this, "general-settings")
          && Object.prototype.hasOwnProperty.call(this, "personalization")
          && generalMessage && typeof generalMessage === "object"
          && typeof generalMessage.id === "string"
          && personalizationMessage && typeof personalizationMessage === "object"
          && typeof personalizationMessage.id === "string"
        ) {
          labelRegistry = this;
          for (const [candidateSlug, candidateMessage] of labelMessages) {
            writePrivateLabel(candidateSlug, candidateMessage);
          }
        }
        return message;
      };
      Object.defineProperty(Object.prototype, slug, {
        configurable: true,
        enumerable: false,
        get: getter
      });
      prototypeLabelHooks.set(slug, { getter, previous });
    };
    const removeLabel = (slug) => {
      labelMessages.delete(slug);
      restorePrototypeLabelHook(slug);
      if (!labelRegistry || !previousLabels.has(slug)) return;
      const current = Object.getOwnPropertyDescriptor(labelRegistry, slug);
      const message = current?.value;
      if (message?.id === "codex-tweaks.settings." + slug) {
        const previous = previousLabels.get(slug);
        if (previous) Object.defineProperty(labelRegistry, slug, previous);
        else delete labelRegistry[slug];
      }
      previousLabels.delete(slug);
    };

    const cleanupMount = (element, record) => {
      try { record?.cleanup?.(); } catch (_) {}
      mounted.delete(element);
    };
    const scanMounts = () => {
      for (const [element, record] of mounted) {
        if (!element.isConnected || renderers.get(record.slug) !== record.renderer) {
          cleanupMount(element, record);
        }
      }
      for (const element of document.querySelectorAll("[data-codex-tweaks-settings-section-host]")) {
        if (mounted.has(element)) continue;
        const slug = element.getAttribute("data-codex-tweaks-settings-section-host");
        const renderer = renderers.get(slug);
        if (!renderer) continue;
        try {
          const cleanup = renderer.mount(element);
          mounted.set(element, {
            slug,
            renderer,
            cleanup: typeof cleanup === "function" ? cleanup : null
          });
        } catch (error) {
          element.setAttribute("data-codex-tweaks-settings-section-error", "");
          console.error("Codex Tweaks settings section failed to mount", error);
        }
      }
    };
    const observer = new MutationObserver(scanMounts);
    const navigateToSettings = (path, replace = false) => {
      if (!navigationBus) return;
      navigationBus.dispatchHostMessage({ type: "navigate-to-route", path, replace });
    };
    const activeSettingsSlug = () => {
      const activeButton = document.querySelector(
        'button[data-settings-panel-slug][aria-current="page"]'
      );
      if (activeButton?.dataset?.settingsPanelSlug) {
        return activeButton.dataset.settingsPanelSlug;
      }
      const activeHost = [...document.querySelectorAll(
        "[data-codex-tweaks-settings-section-host]"
      )].find((element) => element.isConnected);
      return activeHost?.getAttribute("data-codex-tweaks-settings-section-host") ?? null;
    };
    const refreshSettingsRoute = () => {
      const activeSlug = activeSettingsSlug();
      if (activeSlug) navigateToSettings("/settings/" + activeSlug, true);
    };

    const adapter = {
      has(slug) {
        return renderers.has(slug);
      },
      register(packageID, sectionID, mount) {
        const descriptor = [...descriptors.values()].find(
          (candidate) => candidate.packageID === packageID && candidate.id === sectionID
        );
        if (!descriptor) {
          throw new Error("Undeclared settings section: " + sectionID);
        }
        if (typeof mount !== "function") {
          throw new TypeError("Settings section mount must be a function");
        }
        if (!registry || !iconMap || !navigationBus || !settingsRouteChildren || !routeTemplate) {
          throw new Error("Codex settings module is not ready");
        }
        if (renderers.has(descriptor.slug)) {
          throw new Error("Settings section is already registered: " + sectionID);
        }
        const renderer = { mount };
        renderers.set(descriptor.slug, renderer);
        installLabel(descriptor);
        if (!registry.some((entry) => entry?.slug === descriptor.slug)) {
          registry.push({ slug: descriptor.slug });
        }
        installRoute(descriptor);
        if (!previousIcons.has(descriptor.slug)) {
          const previousDescriptor = Object.getOwnPropertyDescriptor(iconMap, descriptor.slug) ?? null;
          previousIcons.set(descriptor.slug, {
            descriptor: previousDescriptor
          });
        }
        Object.defineProperty(iconMap, descriptor.slug, {
          configurable: true,
          enumerable: true,
          writable: true,
          value: iconMap[descriptor.icon] ?? iconMap.personalization
        });
        applyGroupPlacements();
        scanMounts();
        refreshSettingsRoute();
        let registered = true;
        const unregister = () => {
          if (!registered) return;
          registered = false;
          if (renderers.get(descriptor.slug) === renderer) {
            const activeSlug = activeSettingsSlug();
            renderers.delete(descriptor.slug);
            for (const [element, record] of mounted) {
              if (record.slug === descriptor.slug) cleanupMount(element, record);
            }
            for (let index = registry.length - 1; index >= 0; index -= 1) {
              if (registry[index]?.slug === descriptor.slug) registry.splice(index, 1);
            }
            removeRoute(descriptor.slug);
            removeLabel(descriptor.slug);
            const previous = previousIcons.get(descriptor.slug);
            if (previous) {
              if (previous.descriptor) Object.defineProperty(iconMap, descriptor.slug, previous.descriptor);
              else delete iconMap[descriptor.slug];
              previousIcons.delete(descriptor.slug);
            }
            applyGroupPlacements();
            if (activeSlug === descriptor.slug) {
              navigateToSettings("/settings/general-settings", true);
            } else {
              refreshSettingsRoute();
            }
          }
        };
        return Object.freeze({
          id: descriptor.id,
          slug: descriptor.slug,
          open() {
            navigateToSettings("/settings/" + descriptor.slug);
          },
          unregister
        });
      },
      cleanup() {
        const activeSlug = activeSettingsSlug();
        const wasCustomRoute = Boolean(activeSlug && descriptors.has(activeSlug));
        if (wasCustomRoute) {
          navigateToSettings("/settings/general-settings", true);
        }
        observer.disconnect();
        restoreGroupMapInterceptor();
        for (const [element, record] of mounted) cleanupMount(element, record);
        renderers.clear();
        for (const descriptor of descriptors.values()) {
          removeRoute(descriptor.slug);
          removeLabel(descriptor.slug);
        }
        applyGroupPlacements();
        if (registry) {
          if (registry.filter === registryFilter) {
            if (originalRegistryFilterDescriptor) {
              Object.defineProperty(registry, "filter", originalRegistryFilterDescriptor);
            } else {
              delete registry.filter;
            }
          }
          for (let index = registry.length - 1; index >= 0; index -= 1) {
            if (descriptors.has(registry[index]?.slug)) registry.splice(index, 1);
          }
        }
        if (iconMap) {
          for (const [slug, previous] of previousIcons) {
            if (previous.descriptor) Object.defineProperty(iconMap, slug, previous.descriptor);
            else delete iconMap[slug];
          }
        }
        if (!wasCustomRoute) refreshSettingsRoute();
        if (globalThis[globalKey] === adapter) delete globalThis[globalKey];
      }
    };
    globalThis[globalKey] = adapter;

    try {
      const [appModule, visibilityModule] = await Promise.all([
        import(configuration.appModuleUrl),
        import(configuration.visibilityModuleUrl)
      ]);
      registry = Object.values(appModule).find((value) =>
        Array.isArray(value)
        && value.some((entry) => entry?.slug === "general-settings")
        && value.some((entry) => entry?.slug === "personalization")
      );
      if (!registry) throw new Error("Codex settings registry unavailable");
      navigationBus = Object.values(appModule).find((value) =>
        value && typeof value === "object"
        && value.handlers instanceof Map
        && typeof value.dispatchHostMessage === "function"
        && typeof value.deliverMessage === "function"
      );
      if (!navigationBus) throw new Error("Codex navigation bus unavailable");
      const settingsRoute = findSettingsRouteElement();
      settingsRouteChildren = settingsRoute?.props?.children;
      if (!Array.isArray(settingsRouteChildren)) {
        throw new Error("Codex settings route registry unavailable");
      }
      if (Object.isFrozen(settingsRouteChildren)) {
        throw new Error("Codex settings route registry is immutable");
      }
      routeTemplate = flattenReactElements(settingsRouteChildren).find(
        (candidate) => candidate?.props?.path === "*"
      );
      if (!routeTemplate?.type) {
        throw new Error("Codex settings route component unavailable");
      }
      originalRegistryFilterDescriptor = Object.getOwnPropertyDescriptor(registry, "filter") ?? null;
      registryFilter = function (callback, thisArg) {
        const visible = Array.prototype.filter.call(this, callback, thisArg);
        for (const descriptor of descriptors.values()) {
          if (!renderers.has(descriptor.slug)) continue;
          if (!visible.some((entry) => entry?.slug === descriptor.slug)) {
            visible.push(registry.find((entry) => entry?.slug === descriptor.slug) ?? { slug: descriptor.slug });
          }
        }
        return visible;
      };
      registry.filter = registryFilter;
      if (registry.filter !== registryFilter) {
        throw new Error("Codex settings registry is immutable");
      }

      iconMap = Object.values(visibilityModule).find((value) =>
        value && typeof value === "object"
        && typeof value.personalization === "function"
        && typeof value["general-settings"] === "function"
      );
      if (!iconMap) throw new Error("Codex settings icon registry unavailable");
      observer.observe(document.documentElement, { childList: true, subtree: true });
      scanMounts();
      return adapter;
    } catch (error) {
      adapter.cleanup();
      throw error;
    }
  };

  try {
    settingsAdapter = await installSettingsAdapter(settingsAdapterConfiguration);
  } catch (error) {
    settingsAdapterError = error instanceof Error ? error.message : String(error);
    console.error("Codex Tweaks settings adapter unavailable", error);
  }

  const createPackageCapabilities = (token, grants, packageID, registerCleanup) => {
    const handles = new Map();
    for (const [capabilityID, grant] of Object.entries(grants ?? {})) {
      if (capabilityID === "network" && grant?.version === "1.0.0") {
        handles.set(capabilityID, Object.freeze({
          id: capabilityID,
          version: grant.version,
          request(parameters = {}) {
            return invokeCapability(token, capabilityID, "request", parameters);
          }
        }));
      }
      if (
        capabilityID === "ui.settings-section"
        && grant?.version === "1.0.0"
        && settingsAdapter
      ) {
        const declaredSections = Array.isArray(grant.permissions?.sections)
          ? grant.permissions.sections
          : [];
        handles.set(capabilityID, Object.freeze({
          id: capabilityID,
          version: grant.version,
          list() {
            return declaredSections.map((section) => ({
              id: section.id,
              title: section.title,
              slug: section.slug
            }));
          },
          register(options) {
            if (!options || typeof options !== "object") {
              throw new TypeError("Settings section registration must be an object");
            }
            const sectionID = String(options.id ?? "").trim();
            const mount = options.mount;
            const registration = settingsAdapter.register(packageID, sectionID, mount);
            registerCleanup(registration.unregister);
            return registration;
          }
        }));
      }
    }
    const normalizeCapabilityID = (capabilityID) => {
      if (typeof capabilityID !== "string" || !capabilityID.trim()) {
        throw new TypeError("Capability ID must be a non-empty string");
      }
      return capabilityID.trim();
    };
    return Object.freeze({
      has(capabilityID) {
        return handles.has(normalizeCapabilityID(capabilityID));
      },
      get(capabilityID) {
        return handles.get(normalizeCapabilityID(capabilityID));
      },
      require(capabilityID) {
        const normalizedID = normalizeCapabilityID(capabilityID);
        const handle = handles.get(normalizedID);
        if (!handle) {
          throw new Error("Required capability unavailable: " + normalizedID);
        }
        return handle;
      },
      list() {
        return [...handles.keys()];
      }
    });
  };

  const cleanupPackageState = (state) => {
    if (!state || state.cleaned) return;
    state.cleaned = true;
    state.activated = false;
    for (const callback of state.cleanupCallbacks.reverse()) {
      try { callback(); } catch (_) {}
    }
    state.libraries.clear();
    state.style.remove();
    state.root.remove();
  };

  const runtime = {
    version,
    bridgeSessionID,
    settingsAdapterKey,
    settingsAdapterReady: !settingsAdapterExpected || Boolean(settingsAdapter),
    packageErrors,
    settleCapability(response) {
      if (!response || typeof response.id !== "string") return false;
      const pending = capabilityPending.get(response.id);
      if (!pending) return false;
      capabilityPending.delete(response.id);
      clearTimeout(pending.timeout);
      if (response.ok) {
        pending.resolve(response.result);
      } else {
        pending.reject(capabilityError(
          response.error?.message ?? "Capability request failed",
          response.error?.code ?? "capability_error"
        ));
      }
      return true;
    },
    cleanup() {
      for (const state of [...packageStates.values()].reverse()) {
        cleanupPackageState(state);
      }
      packageStates.clear();
      for (const pending of capabilityPending.values()) {
        clearTimeout(pending.timeout);
        pending.reject(capabilityError("Codex Tweaks runtime was cleaned up", "runtime_cleanup"));
      }
      capabilityPending.clear();
      try { settingsAdapter?.cleanup?.(); } catch (_) {}
      settingsAdapter = null;
      host.remove();
      if (globalThis[key] === runtime) delete globalThis[key];
    }
  };
  globalThis[key] = runtime;

%s

%s

  return { status: "injected", version, packageErrors, settingsAdapterError };
})()`, JSONLiteral(effectiveVersion), JSONLiteral(bridgeSessionID), JSONLiteral(settingsAdapterConfiguration), JSONLiteral(capabilityBindingName), strings.Join(setups, "\n"), strings.Join(executions, "\n"))
}

func packageSetupScript(pkg CompiledPackage) string {
	return fmt.Sprintf(`  {
    const packageID = %s;
    const packageName = %s;
    const packageVersion = %s;
    const style = document.createElement("style");
    style.setAttribute("data-codex-tweaks-package-style", packageID);
    style.textContent = %s;
    (document.head || document.documentElement).appendChild(style);

    const root = document.createElement("div");
    root.setAttribute("data-codex-tweaks-package-root", packageID);
    host.appendChild(root);
    packageStates.set(packageID, {
      id: packageID,
      name: packageName,
      version: packageVersion,
      style,
      root,
      cleanupCallbacks: [],
      libraries: new Map(),
      activated: false,
      cleaned: false
    });
  }`, JSONLiteral(pkg.ID), JSONLiteral(pkg.Name), JSONLiteral(pkg.Version), JSONLiteral(pkg.CSS))
}

func packageExecutionScript(pkg CompiledPackage, capabilityToken string) string {
	return fmt.Sprintf(`  {
    const packageID = %s;
    const packageName = %s;
    const packageVersion = %s;
    const dependencyIDs = %s ?? [];
    const grantedCapabilities = %s;
    const capabilityToken = %s;
    const state = packageStates.get(packageID);
    const registerPackageCleanup = (callback) => {
      if (typeof callback === "function") state.cleanupCallbacks.push(callback);
    };
    const normalizeLibraryName = (name) => {
      if (typeof name !== "string" || !name.trim()) {
        throw new TypeError("Library name must be a non-empty string");
      }
      return name.trim();
    };
    const api = {
      packageID,
      version: packageVersion,
      capabilities: createPackageCapabilities(
        capabilityToken,
        grantedCapabilities,
        packageID,
        registerPackageCleanup
      ),
      registerCleanup(callback) {
        registerPackageCleanup(callback);
      },
      registerLibrary(name, value) {
        const normalizedName = normalizeLibraryName(name);
        if (state.libraries.has(normalizedName)) {
          throw new Error(`+"`"+`Library already registered: ${normalizedName}`+"`"+`);
        }
        state.libraries.set(normalizedName, value);
        return value;
      },
      hasLibrary(name) {
        return state.libraries.has(normalizeLibraryName(name));
      },
      getLibrary(name) {
        const normalizedName = normalizeLibraryName(name);
        if (!state.libraries.has(normalizedName)) {
          throw new Error(`+"`"+`Library not found: ${normalizedName}`+"`"+`);
        }
        return state.libraries.get(normalizedName);
      },
      listLibraries() {
        return [...state.libraries.keys()];
      }
    };
    const dependencies = {
      has(dependencyID) {
        return dependencyIDs.includes(dependencyID)
          && packageStates.get(dependencyID)?.activated === true;
      },
      get(dependencyID) {
        if (!dependencyIDs.includes(dependencyID)) {
          throw new Error(`+"`"+`Undeclared package dependency: ${dependencyID}`+"`"+`);
        }
        const dependencyState = packageStates.get(dependencyID);
        if (!dependencyState?.activated) {
          throw new Error(`+"`"+`Package dependency unavailable: ${dependencyID}`+"`"+`);
        }
        return {
          id: dependencyState.id,
          name: dependencyState.name,
          version: dependencyState.version,
          hasLibrary(name) {
            return dependencyState.libraries.has(normalizeLibraryName(name));
          },
          getLibrary(name) {
            const normalizedName = normalizeLibraryName(name);
            if (!dependencyState.libraries.has(normalizedName)) {
              throw new Error(
                `+"`"+`Library not found in ${dependencyID}: ${normalizedName}`+"`"+`
              );
            }
            return dependencyState.libraries.get(normalizedName);
          },
          listLibraries() {
            return [...dependencyState.libraries.keys()];
          }
        };
      },
      list() {
        return [...dependencyIDs];
      }
    };
    const context = {
      id: packageID,
      name: packageName,
      version: packageVersion,
      root: state.root,
      api,
      capabilities: api.capabilities,
      dependencies,
      onCleanup: api.registerCleanup
    };

    try {
      for (const dependencyID of dependencyIDs) {
        if (!packageStates.get(dependencyID)?.activated) {
          throw new Error(`+"`"+`Package dependency unavailable: ${dependencyID}`+"`"+`);
        }
      }
      const module = { exports: {} };
      ((module, exports) => {
%s
      })(module, module.exports);
      const exported = module.exports?.default ?? module.exports;
      const activate = typeof exported === "function"
        ? exported
        : exported?.activate;
      if (typeof activate !== "function") {
        throw new TypeError("Package entry must export activate(context)");
      }
      const cleanup = await activate(context);
      if (typeof cleanup === "function") api.registerCleanup(cleanup);
      state.activated = true;
    } catch (error) {
      cleanupPackageState(state);
      packageStates.delete(packageID);
      const message = error instanceof Error
        ? (error.stack || error.message)
        : String(error);
      packageErrors.push({ id: packageID, name: packageName, message });
    }
  }`, JSONLiteral(pkg.ID), JSONLiteral(pkg.Name), JSONLiteral(pkg.Version), JSONLiteral(pkg.DependencyIDs), JSONLiteral(pkg.Capabilities), JSONLiteral(capabilityToken), pkg.JavaScript)
}
