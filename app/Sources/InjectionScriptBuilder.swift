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

          const api = {
            version,
            registerCleanup(callback) {
              if (typeof callback === "function") cleanupCallbacks.push(callback);
            },
            cleanup() {
              for (const callback of cleanupCallbacks.reverse()) {
                try { callback(); } catch (_) {}
              }
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
