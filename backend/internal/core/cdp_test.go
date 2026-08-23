package core

import "testing"

func TestInjectableTargetsSelectOnlyAppPages(t *testing.T) {
	data := []byte(`[
      {"id":"codex-main","type":"page","title":"Codex","url":"app://-/index.html","webSocketDebuggerUrl":"ws://127.0.0.1:9335/devtools/page/codex-main"},
      {"id":"browser","type":"page","title":"Example","url":"https://example.com","webSocketDebuggerUrl":"ws://127.0.0.1:9335/devtools/page/browser"},
      {"id":"worker","type":"service_worker","title":"Worker","url":"app://-/worker.js","webSocketDebuggerUrl":"ws://127.0.0.1:9335/devtools/page/worker"},
      {"id":"missing","type":"page","title":"Codex","url":"app://-/index.html"}
    ]`)
	targets, err := InjectableTargets(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].ID != "codex-main" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}
