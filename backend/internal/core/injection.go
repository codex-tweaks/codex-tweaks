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
	effectiveVersion := payload.Version
	if forceGeneration != 0 {
		effectiveVersion = fmt.Sprintf("%s-force-%d", payload.Version, forceGeneration)
	}
	setups := make([]string, 0, len(payload.Packages))
	executions := make([]string, 0, len(payload.Packages))
	for _, pkg := range payload.Packages {
		setups = append(setups, packageSetupScript(pkg))
		executions = append(executions, packageExecutionScript(pkg))
	}
	return fmt.Sprintf(`(async () => {
  const key = "__CODEX_TWEAKS__";
  const version = %s;
  const existing = globalThis[key];
  if (
    existing?.version === version &&
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
    packageErrors,
    cleanup() {
      for (const state of [...packageStates.values()].reverse()) {
        cleanupPackageState(state);
      }
      packageStates.clear();
      host.remove();
      if (globalThis[key] === runtime) delete globalThis[key];
    }
  };
  globalThis[key] = runtime;

%s

%s

  return { status: "injected", version, packageErrors };
})()`, JSONLiteral(effectiveVersion), strings.Join(setups, "\n"), strings.Join(executions, "\n"))
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

func packageExecutionScript(pkg CompiledPackage) string {
	return fmt.Sprintf(`  {
    const packageID = %s;
    const packageName = %s;
    const packageVersion = %s;
    const dependencyIDs = %s;
    const state = packageStates.get(packageID);
    const normalizeLibraryName = (name) => {
      if (typeof name !== "string" || !name.trim()) {
        throw new TypeError("Library name must be a non-empty string");
      }
      return name.trim();
    };
    const api = {
      packageID,
      version: packageVersion,
      registerCleanup(callback) {
        if (typeof callback === "function") state.cleanupCallbacks.push(callback);
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
  }`, JSONLiteral(pkg.ID), JSONLiteral(pkg.Name), JSONLiteral(pkg.Version), JSONLiteral(pkg.DependencyIDs), pkg.JavaScript)
}
