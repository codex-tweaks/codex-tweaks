---
name: develop-codex-tweaks-package
description: Create or modify Codex Tweaks API v2 feature packages for the packaged macOS or Windows app, including lifecycle-safe browser injection, independently versioned host capabilities, dependencies, priorities, and Git-installable metadata.
---

# Develop Codex Tweaks Packages

Build one independently loadable feature package for Codex Tweaks while preserving the user's requested scope and the package manager's safety model.

## Working boundary

- Use the packages directory shown by Codex Tweaks as the source of truth. Its default location is `~/Library/Application Support/Codex Tweaks/Tweaks/packages/<package>/` on macOS and `%LOCALAPPDATA%\Codex Tweaks\Tweaks\packages\<package>\` on Windows.
- One direct child directory is one package. The package-level switch controls everything inside it.
- Inspect the target package and relevant Codex page behavior before editing. If the user asks only for review or diagnosis, remain read-only.
- Treat Codex Tweaks as an already installed, packaged host. Do not search for, modify, rebuild, or replace its source or application bundle; keep all edits inside the requested package.
- Use only the package API documented here and capabilities returned by the installed app's current `AppSnapshot.availableCapabilities`. If a required operation is not exposed, report that boundary instead of adding a package-local bridge or patching Codex private modules directly.
- A newly discovered or remotely installed package remains disabled until the user explicitly enables it in the app.

## Manifest contract

Each package must contain `package.json` and a source entry inside the package directory:

```json
{
  "name": "my-tweak",
  "version": "1.0.0",
  "description": "用一句话说明这个包在页面中实现的作用。",
  "type": "module",
  "dependencies": {},
  "codexTweaks": {
    "apiVersion": 2,
    "entry": "src/index.js",
    "priority": 100,
    "packageDependencies": {},
    "capabilities": {}
  }
}
```

- `name` is the stable, unique package ID. Do not rename it casually.
- `version` must be valid SemVer. Increment it only when the task calls for a package release or version change.
- `description` is displayed directly in the package UI and should state the observable purpose.
- `dependencies` contains npm compile-time dependencies. Use explicit versions and commit `package-lock.json` whenever it is non-empty.
- `codexTweaks.entry` is resolved inside the package and compiled by esbuild for the browser.
- `codexTweaks.priority` is the author's default priority; smaller values load earlier. Never rewrite it merely to represent a user's local ordering because the app stores user overrides separately.

## Host capabilities

Before selecting or using a host capability, obtain the latest `AppSnapshot` from the installed Codex Tweaks backend and read `AppSnapshot.availableCapabilities`:

- The `initialize` and `getState` JSON-RPC responses contain the snapshot in `result`; a pushed `state` event contains it in `data`.
- On an already initialized newline-delimited JSON-RPC connection, send `{"id": <request-id>, "method": "getState"}` and read `result.availableCapabilities`.
- Treat the returned array as the only authoritative capability catalog. Follow each returned descriptor's manifest declaration, runtime access, usage, constraints, methods, fields, errors, and examples exactly; do not infer capabilities from the app version or hardcode a catalog from this Skill.
- Keep requests under `codexTweaks.capabilities`. The host validates the manifest and exposes only the approved capability handles through `api.capabilities` / `context.capabilities` at runtime.

## Package dependencies

Declare Codex Tweaks package dependencies under `codexTweaks.packageDependencies`, not npm `dependencies`:

```json
{
  "codexTweaks": {
    "apiVersion": 2,
    "entry": "src/index.js",
    "priority": 100,
    "packageDependencies": {
      "local-core": {
        "version": "^1.0.0"
      },
      "shared-core": {
        "version": "^1.2.0",
        "source": {
          "url": "https://github.com/example/shared-core.git",
          "selector": {
            "type": "latestSemverTag"
          }
        }
      }
    }
  }
}
```

- A dependency is loaded and activated before its dependent package, regardless of user priority.
- The object key is the dependency's canonical package ID and `version` is always required. The app must be able to build the dependency graph without network access.
- Supported version requirements are exact SemVer, `^`, `~`, comparison operators, and `x` or `*` wildcards.
- Omitting `source` declares a local-only dependency. If the package is missing, the app reports it but does not guess or download a repository.
- Providing `source` declares an explicit Git installation source. A compatible package already present locally may satisfy the dependency; otherwise the app can install it after user confirmation and lock the resolved commit.
- A Git URL alone is not a valid canonical dependency declaration. When a UI accepts a repository URL by itself, it must first read the repository package manifest and then persist both the discovered package ID and the Git source.
- A Git-managed dependency still runs from local compiled output. Treat `source` as acquisition and provenance metadata, not as a remote runtime import.
- Do not add an invented repository URL.
- Supported selector types are `branch`, `latestSemverTag`, `tag`, `githubLatestRelease`, `githubRelease`, and `commit`. `branch`, `tag`, `githubRelease`, and `commit` require a `value` except the two `latest` selectors.
- GitHub Release selectors apply only to github.com repositories. Generic Git repositories use branches, tags, or commits.
- Do not clone repositories, run Git, download dependencies, or self-update from package runtime code. The app owns resolution, confirmation, locking, downloading, and updates.
- Avoid circular dependencies. If several packages require incompatible versions of the same package ID, report the conflict rather than bypassing it.
- User priority only orders packages that are not constrained by a dependency path. The app keeps that override separately, removes it when it equals the declared priority, and explains when dependency topology overrides the requested numeric order.

Package dependencies provide lifecycle ordering and namespaced runtime capabilities. They do not make another package's source importable at compile time. Shared compile-time code belongs in an npm package.

## Entry and lifecycle

The entry must export `activate(context)` and may return a cleanup function:

```js
import "./style.css";

export function activate({ root, onCleanup, api, dependencies }) {
  const element = document.createElement("div");
  root.append(element);

  const handleEvent = () => {};
  window.addEventListener("resize", handleEvent);
  onCleanup(() => window.removeEventListener("resize", handleEvent));

  api.registerLibrary("example", { element });
  const shared = dependencies.get("shared-core").getLibrary("public-api");
  void shared;
}
```

The context contains:

- `id`, `name`, and `version` for the active compiled package.
- `root`, an isolated DOM root owned by the package.
- `onCleanup(callback)` and `api.registerCleanup(callback)` for teardown.
- `api.capabilities` / `capabilities` for independently versioned host capabilities declared in the manifest.
- `api.registerLibrary`, `hasLibrary`, `getLibrary`, and `listLibraries` for libraries exported by this package.
- `dependencies.has`, `get`, and `list` for declared dependency packages and their exported libraries. Access is limited to dependencies named in the manifest.

Every observer, timer, event listener, DOM node mounted outside `root`, and external state mutation must be reversible. Activation and cleanup must tolerate reinjection, page refreshes, and target elements appearing later. A package failure must not prevent unrelated packages from running.

## Browser and style constraints

- Source may use JavaScript, TypeScript, `import`/`export`, npm modules, and CSS imports supported by esbuild.
- Page runtime has no Node built-ins. Do not use `fs`, `child_process`, Electron internals, or runtime npm installation.
- Do not load scripts, CSS, fonts, or modules from a CDN at runtime; bundle required code locally.
- Scope owned DOM and CSS with a unique `data-codex-tweaks-*` attribute or package-specific class.
- Prefer stable roles, ARIA attributes, text meaning, and structure over generated class names when locating Codex UI.
- Handle light and dark appearance, viewport edges, long text, keyboard focus, and `prefers-reduced-motion` when relevant.
- Decorative or watermark-only UI must use `pointer-events: none` and must not register click behavior.

## Build and verification

For local distribution, provide either the package directory itself or a ZIP with `package.json` at its root or inside one unambiguous top-level directory. Do not include `.git`, `node_modules`, symbolic links, or special files. Keep `package-lock.json` beside `package.json` whenever npm `dependencies` is non-empty. Local installation validates the complete package in staging and never overwrites an installed package with the same canonical ID.

- The normal update path runs `npm ci --ignore-scripts` for locked npm dependencies and then the app-pinned esbuild version.
- Developer mode may compile local source changes from existing dependencies and compiler cache, but must not silently fetch Git updates or new npm dependencies.
- Validate `package.json`, dependency IDs and ranges, entry containment, and cleanup behavior.
- Use the Codex Tweaks package page to perform a manual compile when runtime verification is requested and available.
- Run relevant non-destructive project tests. Distinguish source/test evidence from actual visual observation in Codex.
- Report the package changed, manifest or dependency changes, compilation/tests performed, and any page behavior still requiring user confirmation.
