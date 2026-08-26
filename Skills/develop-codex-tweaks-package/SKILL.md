---
name: develop-codex-tweaks-package
description: Create or modify Codex Tweaks API v3 packages with lifecycle-safe Renderer code, optional fully trusted Node backends, typed UI extensions, package dependencies, priorities, and Git-installable metadata.
---

# Develop Codex Tweaks Packages

Build one independently loadable Codex Tweaks package while preserving the user's requested scope and the host's trust model.

## Working boundary

- Use the packages directory shown by Codex Tweaks as the source of truth. Its default location is `~/Library/Application Support/Codex Tweaks/Tweaks/packages/<package>/` on macOS and `%LOCALAPPDATA%\Codex Tweaks\Tweaks\packages\<package>\` on Windows.
- One direct child directory is one package. Its switch controls the Renderer and Node parts together.
- Inspect the target package and relevant Codex behavior before editing. Stay read-only when the user asks only for review or diagnosis.
- Treat Codex Tweaks as an installed host. Do not modify, rebuild, or replace its application bundle; keep requested package work inside the package directory.
- A newly discovered or remotely installed package remains disabled until the user enables it.
- Use API v3 only. Do not add API v2 compatibility, `codexTweaks.capabilities`, `api.capabilities`, hidden DOM bridges, localStorage queues, or direct imports of Codex private modules.

## Manifest contract

Every package contains `package.json` and a Renderer entry:

```json
{
  "name": "ct-my-tweak",
  "version": "1.0.0",
  "description": "用一句话说明这个包在页面中实现的作用。",
  "type": "module",
  "repository": {
    "type": "git",
    "url": "https://github.com/example/codex-tweaks-my-tweak.git"
  },
  "homepage": "https://github.com/example/codex-tweaks-my-tweak",
  "keywords": ["codex-tweaks-package", "codex-tweaks-ui"],
  "license": "MIT",
  "dependencies": {},
  "codexTweaks": {
    "apiVersion": 3,
    "entrypoints": {
      "renderer": "src/index.js"
    },
    "priority": 100,
    "packageDependencies": {}
  }
}
```

- `name` is the stable unique package ID. For a shared package, use `ct-<slug>` and do not rename it after publication.
- `version` must be valid SemVer. Increment it only when the task calls for a release or version change.
- `description` is shown directly to users and should state observable behavior.
- `dependencies` contains npm dependencies used at build or Node runtime. Pin explicit versions and commit `package-lock.json` whenever it is non-empty.
- `codexTweaks.entrypoints.renderer` stays inside the package and is bundled for the browser.
- `codexTweaks.priority` is the author's default; smaller values activate earlier. User ordering is stored separately by the app.

## Public repository and naming convention

Use this convention whenever the user wants to publish or share a package through Git:

- Keep one Codex Tweaks package in one repository, with `package.json` and `README.md` at the repository root.
- Name both the package source directory and Git repository `codex-tweaks-<slug>`, for example `codex-tweaks-compact-sidebar`. The slug uses lowercase ASCII letters, digits, and single hyphens.
- Name the package ID in `package.json` `ct-<slug>`, for example `ct-compact-sidebar`. Use the same slug as the directory and repository, choose a distinctive slug to avoid collisions, and keep the ID unchanged after publication.
- Add the GitHub topic `codex-tweaks-package`. Add focused topics such as `codex-tweaks-theme`, `codex-tweaks-layout`, `codex-tweaks-workflow`, or `codex-tweaks-node` only when they describe the package.
- Keep `repository`, `homepage`, `keywords`, and `license` accurate. Never imply that a third-party package is official, reviewed, or trusted merely because it follows the naming convention.
- Publish immutable `v<SemVer>` tags, for example `v1.2.0`. Prefer `latestSemverTag` for normal Git installation and updates.
- Treat this as a public-discovery convention, not an installation restriction. Existing packages, private repositories, and non-GitHub Git hosts do not need to be renamed merely to remain installable.

## Package README

Every package should include a root `README.md`, including local-only packages. Write it for the person deciding whether to install and trust the package, not only for its developer. Use this template and remove sections that genuinely do not apply:

```md
# <Package display name>

<One sentence describing the observable change this package makes in Codex.>

## Features

- <User-visible behavior>
- <Another user-visible behavior>

## Install

Install this repository from the Codex Tweaks package page and select a released `v<SemVer>` tag. New packages remain disabled until you review and enable them.

## Permissions and safety

- Renderer access: <what page content the package reads or changes>
- Node access: <why Node is required and what local resources it uses, or "Not used">
- Network access: <destinations and purpose, or "Not used">

## Compatibility

- Codex Tweaks API: v3
- Tested platforms: <macOS, Windows, or both>
- Known limitations: <limitations or "None known">

## Development

<Commands for installing dependencies, building, and verifying the package.>

## License

<License name and link to the license file.>
```

## Renderer entry and lifecycle

The Renderer entry exports `activate(context)` and may return a cleanup function:

```js
import "./style.css";

export function activate({ root, onCleanup, api, dependencies, ui, node }) {
  const element = document.createElement("div");
  root.append(element);

  const handleResize = () => {};
  window.addEventListener("resize", handleResize);
  onCleanup(() => window.removeEventListener("resize", handleResize));

  api.registerLibrary("example", { element });
  const shared = dependencies.get("shared-core").getLibrary("public-api");
  void shared;
  void ui;
  void node;
}
```

The context contains:

- `id`, `name`, and `version` for the active compiled package.
- `root`, the package-owned DOM root.
- `onCleanup(callback)` and `api.registerCleanup(callback)` for teardown.
- `api.registerLibrary`, `hasLibrary`, `getLibrary`, and `listLibraries` for package exports.
- `dependencies.has`, `get`, and `list` for package dependencies declared in the manifest.
- `ui`, the typed Renderer UI extension host. Only extensions declared by the package are populated.
- `node`, present only when this exact package revision declared Node, was trusted, and its Node runtime is running.

Every observer, timer, listener, DOM node outside `root`, and external mutation must be reversible. Activation and cleanup must tolerate reinjection, remounting, and late target discovery. One package failure must not prevent unrelated packages from activating.

## Node backend and trust

Declare Node only when browser code and typed UI extensions cannot implement the requirement. A Node declaration grants the package full code execution as the current user, including filesystem, network, local services, databases, child processes, and installed npm modules.

The Node entry and permission must always be declared together:

```json
{
  "codexTweaks": {
    "apiVersion": 3,
    "entrypoints": {
      "renderer": "src/index.js",
      "node": "src/node.js"
    },
    "priority": 100,
    "packageDependencies": {},
    "permissions": {
      "node": {
        "reason": "读取用户选择的图片、生成缩略图，并把结果写入包的数据目录。"
      }
    }
  }
}
```

- `permissions.node.reason` is mandatory, is shown verbatim in the authorization dialog, and must be a concrete 1–1000 character explanation of why Node is needed and what local resources are used.
- Do not write vague reasons such as “improve functionality.” Name filesystem, network, process, database, or local-service use that matters to the user.
- The entire package is blocked until its current Node revision is trusted. Authorization is invalidated by a version, manifest, reason, source, dependency, entry, compiler, or Node bundle change, and when the package is turned off.
- The app may offer an explicitly warned developer-only automatic trust switch. It is off after every app restart and is not a package guarantee; packages must work with the normal authorization flow.
- npm install scripts are suppressed during the pre-authorization build. After authorization, a locked dependency install may execute lifecycle scripts before the Node process starts. Mention material lifecycle-script behavior in the reason.
- Never ask the user to enable Node merely to bypass a missing Renderer convenience API.

The Node entry exports `activate(context)`. Register named RPC handlers and return cleanup when necessary:

```js
import fs from "node:fs/promises";
import path from "node:path";

export function activate({ rpc, packageDirectory, dataDirectory, signal }) {
  rpc.handle("read-config", async ({ name }) => {
    const file = path.join(dataDirectory, `${name}.json`);
    return JSON.parse(await fs.readFile(file, "utf8"));
  });

  const timer = setInterval(() => rpc.emit("tick", { at: Date.now() }), 1000);
  signal.addEventListener("abort", () => clearInterval(timer), { once: true });
  return () => clearInterval(timer);
}
```

Renderer code calls only its own Node process:

```js
export function activate({ node, onCleanup }) {
  if (!node) throw new Error("Node runtime is required");
  node.invoke("read-config", { name: "settings" }).then(console.log);
  const unsubscribe = node.on("tick", (event) => console.log(event.at));
  onCleanup(unsubscribe);
}
```

- RPC parameters, results, and events must be JSON-serializable.
- `rpc.handle(method, handler)` names must be unique inside the package.
- `rpc.emit(name, payload)` broadcasts to active Renderer instances of the same package.
- Use `dataDirectory` for mutable private package data and `packageDirectory` only when the source package itself must be inspected.
- Observe `signal` and clean up files, watchers, servers, timers, subprocesses, and database handles promptly.

## Typed UI extensions

UI integrations are parallel, independently versioned declarations under `codexTweaks.ui`. Do not invent a generic UI capability. Use only documented extension keys.

To add a real Codex settings route:

```json
{
  "codexTweaks": {
    "ui": {
      "settingsSections": {
        "apiVersion": 1,
        "required": false,
        "items": [{
          "id": "appearance-extra",
          "title": "扩展外观",
          "group": "personal",
          "icon": "personalization",
          "after": "personalization"
        }]
      }
    }
  }
}
```

```js
export function activate({ ui }) {
  const settings = ui.settingsSections;
  if (!settings) return;
  return settings.register({
    id: "appearance-extra",
    mount(container) {
      const page = document.createElement("div");
      page.textContent = "扩展外观";
      container.append(page);
      return () => page.remove();
    },
  }).unregister;
}
```

- `settingsSections.apiVersion` must be `1`; one package may declare 1–8 unique lowercase kebab-case items.
- `required` defaults to `true`. Set it to `false` when the package can still provide useful behavior in renderers where the Codex settings adapter is unavailable.
- Valid groups are `personal`, `integrations`, `coding`, and `archived`. Defaults are `personal` and the `personalization` icon.
- The host owns private-module discovery, navigation placement, route registration, and cleanup. Package code owns only the mounted page content.
- `mount` may run again and must return idempotent cleanup for every owned side effect.

## Package dependencies

Declare Codex Tweaks package dependencies under `codexTweaks.packageDependencies`, not npm `dependencies`:

```json
{
  "codexTweaks": {
    "apiVersion": 3,
    "entrypoints": { "renderer": "src/index.js" },
    "priority": 100,
    "packageDependencies": {
      "local-core": { "version": "^1.0.0" },
      "shared-core": {
        "version": "^1.2.0",
        "source": {
          "url": "https://github.com/example/shared-core.git",
          "selector": { "type": "latestSemverTag" }
        }
      }
    }
  }
}
```

- Dependencies activate before dependents. The app blocks dependents when a dependency is missing, disabled, invalid, incompatible, unauthorized, or cyclic.
- Supported version requirements are exact SemVer, `^`, `~`, comparisons, and `x` or `*` wildcards.
- Omitting `source` means local-only. Supplying `source` declares explicit Git acquisition metadata; the app confirms, locks, installs, and updates it.
- Supported selectors are `defaultBranch`, `branch`, `latestSemverTag`, `tag`, `githubLatestRelease`, `githubRelease`, and `commit`. `defaultBranch` follows the repository's remote HEAD; required selector values must be explicit.
- Do not clone, download, self-update, or invent repository URLs from runtime code.
- Package dependencies provide activation ordering and exported libraries, not compile-time source imports. Put shared compile-time code in an npm package.

## Browser and style constraints

- Renderer source may use JavaScript, TypeScript, npm modules, CSS imports, and esbuild-supported syntax.
- Renderer has no Node built-ins. Put required local work in a declared Node entry and call it through `node.invoke`.
- Do not load runtime scripts, CSS, fonts, or modules from a CDN; bundle code locally.
- Scope DOM and CSS with a unique `data-codex-tweaks-*` attribute or package-specific class.
- Prefer stable roles, ARIA attributes, text meaning, and structure over generated class names.
- Handle light/dark appearance, viewport edges, long text, keyboard focus, and reduced motion.
- Decorative UI must use `pointer-events: none` and must not register clicks.

## Build and verification

For local distribution, provide the package directory or a ZIP with `package.json` at its root or under one unambiguous top-level directory. Do not include `.git`, `node_modules`, symbolic links, or special files.

- Keep `package-lock.json` beside `package.json` whenever npm dependencies are non-empty.
- Validate API v3 manifest structure, both entry paths, Node reason, UI declarations, dependency IDs/ranges, and cleanup behavior.
- The normal pre-authorization build uses locked dependencies with lifecycle scripts disabled and the host-pinned esbuild version.
- Developer mode may compile existing local source changes but must not silently trust Node, fetch Git updates, or install new npm dependencies.
- Use the Codex Tweaks package page for build and authorization when live verification is requested and available.
- Run relevant non-destructive tests. Distinguish source/test evidence from actual visual observation in Codex.
- Report package files changed, Node/UI/dependency declarations, build/tests performed, and behavior still requiring user confirmation.
