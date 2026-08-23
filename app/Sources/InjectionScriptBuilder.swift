import Foundation

enum InjectionScriptBuilder {
    static func injectionScript(payload: TweakPayload, forceGeneration: Int) -> String {
        let effectiveVersion = forceGeneration == 0
            ? payload.version
            : "\(payload.version)-force-\(forceGeneration)"
        let setupScripts = payload.packages.map(packageSetupScript).joined(separator: "\n")
        let executionScripts = payload.packages.map(packageExecutionScript).joined(separator: "\n")

        return """
        (async () => {
          const key = "__CODEX_TWEAKS__";
          const version = \(jsonLiteral(effectiveVersion));
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

        \(setupScripts)

        \(executionScripts)

          return { status: "injected", version, packageErrors };
        })()
        """
    }

    static let cleanupScript = """
    (() => {
      const key = "__CODEX_TWEAKS__";
      try { globalThis[key]?.cleanup?.(); } catch (_) {}
      document.getElementById("codex-tweaks-root")?.remove();
      delete globalThis[key];
      return { status: "cleaned" };
    })()
    """

    private static func packageSetupScript(_ package: CompiledTweakPackage) -> String {
        """
          {
            const packageID = \(jsonLiteral(package.id));
            const packageName = \(jsonLiteral(package.name));
            const packageVersion = \(jsonLiteral(package.version));
            const style = document.createElement("style");
            style.setAttribute("data-codex-tweaks-package-style", packageID);
            style.textContent = \(jsonLiteral(package.css));
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
          }
        """
    }

    private static func packageExecutionScript(_ package: CompiledTweakPackage) -> String {
        """
          {
            const packageID = \(jsonLiteral(package.id));
            const packageName = \(jsonLiteral(package.name));
            const packageVersion = \(jsonLiteral(package.version));
            const dependencyIDs = \(jsonLiteral(package.dependencyIDs));
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
                  throw new Error(`Library already registered: ${normalizedName}`);
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
                  throw new Error(`Library not found: ${normalizedName}`);
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
                  throw new Error(`Undeclared package dependency: ${dependencyID}`);
                }
                const dependencyState = packageStates.get(dependencyID);
                if (!dependencyState?.activated) {
                  throw new Error(`Package dependency unavailable: ${dependencyID}`);
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
                        `Library not found in ${dependencyID}: ${normalizedName}`
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
                  throw new Error(`Package dependency unavailable: ${dependencyID}`);
                }
              }
              const module = { exports: {} };
              ((module, exports) => {
        \(package.javascript)
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
          }
        """
    }

    private static func jsonLiteral(_ value: String) -> String {
        let data = try! JSONEncoder().encode(value)
        return String(decoding: data, as: UTF8.self)
    }

    private static func jsonLiteral(_ value: [String]) -> String {
        let data = try! JSONEncoder().encode(value)
        return String(decoding: data, as: UTF8.self)
    }
}
