package core

import (
	"os/exec"
	"strings"
	"testing"
)

func runSettingsJavaScript(t *testing.T, script string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node.js is required for settings adapter tests")
	}
	command := exec.Command(node, "-e", script)
	configureCommand(command)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("settings adapter: %v\n%s", err, output)
	}
}

func settingsInjectionFragment(t *testing.T, start, end string) string {
	t.Helper()
	script := InjectionScript(Payload{Version: "settings-test"}, 0)
	_, tail, ok := strings.Cut(script, start)
	if !ok {
		t.Fatalf("settings fragment start missing: %s", start)
	}
	fragment, _, ok := strings.Cut(tail, end)
	if !ok {
		t.Fatalf("settings fragment end missing: %s", end)
	}
	return start + fragment
}

func TestSettingsGroupsAndVisibilityProjection(t *testing.T) {
	groups := settingsInjectionFragment(t, "const isSettingsGroupRegistry =", "if (typeof originalArrayMap")
	projection := settingsInjectionFragment(t, "registryMap = function", "Object.defineProperty(registry, \"map\"")
	runSettingsJavaScript(t, `const assert = require("node:assert/strict");
const descriptors = new Map([
  ["background", {slug:"background", group:"personal", after:"personalization"}],
  ["archive-tool", {slug:"archive-tool", group:"archived", after:"archived-chats"}]
]);
const routeElements = new Map([["background",{}],["archive-tool",{}]]);
let groupRegistry = null;
`+groups+`
for (const [personal, archived] of [["personal","archived"],["app","data-privacy"]]) {
  groupRegistry = [
    {key:"profile",slugs:["profile"]},
    {key:personal,slugs:["general-settings","personalization","pets"]},
    {key:archived,slugs:["import","archived-chats","storage"]},
    {key:"integrations",slugs:["plugins-settings"]},
    {key:"coding",slugs:["hooks-settings"]}
  ];
  assert.equal(isSettingsGroupRegistry(groupRegistry), true);
  applyGroupPlacements();
  applyGroupPlacements();
  assert.deepEqual(groupRegistry[1].slugs, ["general-settings","personalization","background","pets"]);
  assert.deepEqual(groupRegistry[2].slugs, ["import","archived-chats","archive-tool","storage"]);
  routeElements.clear();
  applyGroupPlacements();
  assert.deepEqual(groupRegistry[1].slugs, ["general-settings","personalization","pets"]);
  assert.deepEqual(groupRegistry[2].slugs, ["import","archived-chats","storage"]);
  routeElements.set("background",{}); routeElements.set("archive-tool",{});
}
assert.equal(isSettingsGroupRegistry([{key:"app",slugs:["personalization"]}]), false);
const originalArrayMap = Array.prototype.map;
let registryMap;
`+projection+`
const entries = [{slug:"general-settings"},{slug:"background"}];
for (const error of [false, undefined]) {
  const result = registryMap.call(entries, entry => {
    if (entry.slug === "background") throw new Error("unknown native settings section");
    return {section:entry,visible:true,pending:false,error};
  });
  assert.deepEqual(result[1], {section:entries[1],visible:true,pending:false,error:false});
}
assert.deepEqual(registryMap.call(entries, entry => entry.slug), ["general-settings","background"]);
`)
}

func TestSettingsRoutesIncludeModalAndMainRouter(t *testing.T) {
	routes := settingsInjectionFragment(t, "const initializeRoutes =", "const labelMessage =")
	runSettingsJavaScript(t, `const assert = require("node:assert/strict");
const discover = (`+settingsRouteRegistryDiscovery+`);
const makeRoute = id => ({id,path:"/settings",children:[
  {id:id+"general",path:"general-settings"},
  {id:id+"personal",path:"personalization"},
  {id:id+"wildcard",path:"*",element:{$$typeof:Symbol.for("react.transitional.element"),type:()=>null,props:{}}}
]});
const main = makeRoute("main");
const modal = makeRoute("modal");
const wrapper = {id:"wrapper",element:{props:{settingsRoutes:[modal]}},children:[main]};
const scope = {object:{routes:[wrapper],duplicate:[main],unrelated:[{path:"/settings"}]}};
assert.equal(discover.call(scope,"routes"), true);
assert.equal(globalThis.routes.registries.length, 2);
assert.equal(globalThis.routes.registries[0].children, main.children);
assert.equal(globalThis.routes.registries[1].children, modal.children);
const configuration = {routeRegistryKey:"routes"};
const routeElements = new Map();
let settingsRouteChildren = null;
let settingsRouteRegistries = [];
`+routes+`
initializeRoutes();
const descriptor = {slug:"background",title:"Background"};
installRoute(descriptor);
installRoute(descriptor);
for (const route of [main,modal]) {
  assert.deepEqual(route.children.map(r=>r.path), ["general-settings","personalization","background","*"]);
  assert.equal(route.children[2].element.props["data-codex-tweaks-settings-section-host"], "background");
}
removeRoute("background");
for (const route of [main,modal]) assert.deepEqual(route.children.map(r=>r.path), ["general-settings","personalization","*"]);
assert.equal(routeElements.size, 0);
// Legacy builds expose a single table. Both install and cleanup still work.
globalThis.routes = {children:main.children,template:main.children.at(-1)};
initializeRoutes(); installRoute(descriptor); removeRoute("background");
assert.equal(main.children.length, 3);
delete globalThis.rejected;
Object.freeze(modal.children);
assert.equal(discover.call(scope,"rejected"), false);
assert.equal(globalThis.rejected, undefined);
`)
}
