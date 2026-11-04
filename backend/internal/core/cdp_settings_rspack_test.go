package core

import (
	"os/exec"
	"testing"
)

func TestRspackSettingsDiscovery(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node.js is required; install the tools declared in mise.toml")
	}
	// Only matching settings modules may be instantiated. Discovery must wait
	// for async startup, and reject missing or ambiguous settings factories.
	script := `const assert = require("node:assert/strict");
const discover = (` + rspackSettingsModuleDiscovery + `);
(async () => {
const modules = {
  registry() { return [{slug:"general-settings"},{slug:"personalization"}]; },
  bus() { class Bus {
    getInstance() { return this; }
    dispatchHostMessage(message) { this.handlers; this.bufferedMessages; }
  } },
  icons() { return {"general-settings":{component:1},personalization:{component:2}}; },
  routes() { return [{path:"/settings",children:[{path:"*"}]}]; },
  panels() { return modules.SettingsLayout; },
  unrelated() { throw new Error("must not execute unrelated modules"); }
};
const exports = {
  registry: {sections:[{slug:"general-settings"},{slug:"personalization"}]},
  bus: {bus:{handlers:new Map(),dispatchHostMessage(){}}},
  icons: {}, routes: {}, unrelated: Object.defineProperty({}, "secret", {
    enumerable:true, get() { throw new Error("must not inspect unrelated exports"); }
  })
};
const loaded = [];
let allowIcons = true;
const iconFactory = modules.icons;
delete modules.icons;
const layout = function(){};
layout.prefetch = async () => { if (allowIcons) runtime.m.icons = iconFactory; };
const runtime = id => {
  if (!runtime.c[id]) {
    loaded.push(id);
    runtime.c[id] = {exports: exports[id]};
  }
  return runtime.c[id].exports;
};
runtime.m = {boot(e,t,r) { r.a(e,async function() {}); }};
runtime.c = {boot:{exports:Promise.resolve().then(() => {
  Object.assign(runtime.m, modules); runtime.c.panels = {exports:{SettingsLayout:layout}};
})}};
assert.deepEqual(await discover(runtime), {registry:"registry",navigation:"bus",icons:"icons",routes:"routes"});
assert.deepEqual(loaded, ["registry", "bus"]);
runtime.m.duplicate = modules.registry;
runtime.c.duplicate = {exports:exports.registry};
await assert.rejects(()=>discover(runtime), /registry module matches: 2/);
delete runtime.m.duplicate;
assert.equal((await discover(runtime)).registry, "registry");
delete runtime.m.icons;
allowIcons = false;
await assert.rejects(()=>discover(runtime), /icons module matches: 0/);
})().catch(error => { console.error(error); process.exitCode = 1; });
`
	command := exec.Command(node, "-e", script)
	configureCommand(command)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Rspack module discovery: %v\n%s", err, output)
	}
}

func TestSettingsModuleScopeRestrictsFactoryAccess(t *testing.T) {
	for _, test := range []struct {
		description string
		moduleID    string
		want        bool
	}{
		{"Module", "", true},
		{"Closure (icons)", "icons", true},
		{"Closure (unrelated)", "icons", false},
		{"Closure (icons)", "", false},
		{"Global", "icons", false},
	} {
		if got := settingsModuleScope(test.description, test.moduleID); got != test.want {
			t.Errorf("settingsModuleScope(%q, %q) = %v", test.description, test.moduleID, got)
		}
	}
}
