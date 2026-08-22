import Foundation

enum InjectionScriptBuilder {
    static func injectionScript(payload: TweakPayload, forceGeneration: Int) -> String {
        let effectiveVersion = forceGeneration == 0
            ? payload.version
            : "\(payload.version)-force-\(forceGeneration)"

        return """
        (() => {
          const key = "__CODEX_TWEAKS__";
          const version = \(jsonLiteral(effectiveVersion));
          const existing = globalThis[key];
          if (
            existing?.version === version &&
            document.getElementById("codex-tweaks-style") &&
            document.getElementById("codex-tweaks-root")
          ) {
            return { status: "unchanged", version };
          }

          try { existing?.cleanup?.(); } catch (_) {}
          document.getElementById("codex-tweaks-style")?.remove();
          document.getElementById("codex-tweaks-root")?.remove();

          const style = document.createElement("style");
          style.id = "codex-tweaks-style";
          style.textContent = \(jsonLiteral(payload.css));
          (document.head || document.documentElement).appendChild(style);

          const host = document.createElement("div");
          host.id = "codex-tweaks-root";
          (document.body || document.documentElement).appendChild(host);
          const root = host.attachShadow({ mode: "open" });
          const cleanupCallbacks = [];
          const libraries = new Map();
          const normalizeLibraryName = (name) => {
            if (typeof name !== "string" || !name.trim()) {
              throw new TypeError("Library name must be a non-empty string");
            }
            return name.trim();
          };

          const api = {
            version,
            registerLibrary(name, value) {
              const normalizedName = normalizeLibraryName(name);
              if (libraries.has(normalizedName)) {
                throw new Error(`Codex Tweaks library already registered: ${normalizedName}`);
              }
              libraries.set(normalizedName, value);
              return value;
            },
            hasLibrary(name) {
              return libraries.has(normalizeLibraryName(name));
            },
            getLibrary(name) {
              const normalizedName = normalizeLibraryName(name);
              if (!libraries.has(normalizedName)) {
                throw new Error(`Codex Tweaks library not found: ${normalizedName}`);
              }
              return libraries.get(normalizedName);
            },
            listLibraries() {
              return [...libraries.keys()];
            },
            runModule(name, initializer) {
              if (typeof initializer !== "function") {
                throw new TypeError("Module initializer must be a function");
              }
              try {
                return initializer.call(globalThis, api, root);
              } catch (error) {
                const detail = error instanceof Error ? error.message : String(error);
                const moduleError = new Error(
                  `Codex Tweaks module failed (${name}): ${detail}`
                );
                moduleError.name = "CodexTweaksModuleError";
                moduleError.moduleName = name;
                moduleError.cause = error;
                throw moduleError;
              }
            },
            registerCleanup(callback) {
              if (typeof callback === "function") cleanupCallbacks.push(callback);
            },
            cleanup() {
              for (const callback of cleanupCallbacks.reverse()) {
                try { callback(); } catch (_) {}
              }
              libraries.clear();
              style.remove();
              host.remove();
              if (globalThis[key] === api) delete globalThis[key];
            }
          };
          globalThis[key] = api;

          try {
            ((api, root) => {
        \(payload.javascript)
            })(api, root);
          } catch (error) {
            api.cleanup();
            throw error;
          }

          return { status: "injected", version };
        })()
        """
    }

    static let cleanupScript = """
    (() => {
      const key = "__CODEX_TWEAKS__";
      try { globalThis[key]?.cleanup?.(); } catch (_) {}
      document.getElementById("codex-tweaks-style")?.remove();
      document.getElementById("codex-tweaks-root")?.remove();
      delete globalThis[key];
      return { status: "cleaned" };
    })()
    """

    private static func jsonLiteral(_ value: String) -> String {
        let data = try! JSONEncoder().encode(value)
        return String(decoding: data, as: UTF8.self)
    }
}
