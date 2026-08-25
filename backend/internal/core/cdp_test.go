package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

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

func TestInjectUsesSmallProbeAfterInitialRuntimeSetup(t *testing.T) {
	const packageMarker = "full-package-bundle-marker"
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	var mutex sync.Mutex
	expressions := []string{}
	runtimeCurrent := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/json/list":
			debuggerURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/page/codex-main"
			_ = json.NewEncoder(writer).Encode([]CDPTarget{{
				ID: "codex-main", Type: "page", URL: "app://-/index.html", WebSocketDebuggerURL: &debuggerURL,
			}})
		case "/devtools/page/codex-main":
			connection, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				return
			}
			defer connection.Close()
			command := map[string]any{}
			if connection.ReadJSON(&command) != nil {
				return
			}
			params, _ := command["params"].(map[string]any)
			expression, _ := params["expression"].(string)
			mutex.Lock()
			expressions = append(expressions, expression)
			isFullInjection := strings.Contains(expression, packageMarker)
			status := "stale"
			if isFullInjection {
				runtimeCurrent = true
				status = "injected"
			} else if runtimeCurrent {
				status = "unchanged"
			}
			mutex.Unlock()
			_ = connection.WriteJSON(map[string]any{
				"id": command["id"],
				"result": map[string]any{"result": map[string]any{"value": map[string]any{
					"status": status, "packageErrors": []any{},
				}}},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	service := NewCDPService(nil)
	service.Endpoint = server.URL + "/json/list"
	service.AllowedOrigin = server.URL
	payload := Payload{Version: "stable", Packages: []CompiledPackage{{
		ID: "sample", Name: "sample", Version: "1.0.0",
		JavaScript: "module.exports.activate = () => {}; /* " + packageMarker + " */",
	}}}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := service.Inject(context.Background(), payload, 0)
		if err != nil {
			t.Fatal(err)
		}
		if result.TargetCount != 1 || result.SuccessCount != 1 || result.FailureCount() != 0 {
			t.Fatalf("unexpected injection result on attempt %d: %#v", attempt+1, result)
		}
	}

	mutex.Lock()
	defer mutex.Unlock()
	if len(expressions) != 3 {
		t.Fatalf("CDP evaluate count = %d, want probe + initial injection + probe", len(expressions))
	}
	if strings.Contains(expressions[0], packageMarker) || !strings.Contains(expressions[1], packageMarker) || strings.Contains(expressions[2], packageMarker) {
		t.Fatalf("unexpected probe/full injection sequence: %#v", expressions)
	}
	if len(expressions[2]) >= len(expressions[1])/4 {
		t.Fatalf("unchanged runtime probe is not lightweight: probe=%d full=%d", len(expressions[2]), len(expressions[1]))
	}
}

func TestReloadAllTargetsTerminatesExecutionAndReloadsAppPages(t *testing.T) {
	commandChannel := make(chan map[string]any, 2)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/json/list":
			debuggerURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/page/codex-main"
			_ = json.NewEncoder(writer).Encode([]CDPTarget{
				{ID: "codex-main", Type: "page", URL: "app://-/index.html", WebSocketDebuggerURL: &debuggerURL},
				{ID: "external", Type: "page", URL: "https://example.com", WebSocketDebuggerURL: &debuggerURL},
			})
		case "/devtools/page/codex-main":
			connection, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				return
			}
			defer connection.Close()
			var command map[string]any
			if err := connection.ReadJSON(&command); err != nil {
				return
			}
			commandChannel <- command
			if command["method"] == "Runtime.terminateExecution" {
				_ = connection.WriteJSON(map[string]any{
					"id": command["id"], "error": map[string]any{"message": "unsupported in test"},
				})
				return
			}
			_ = connection.WriteJSON(map[string]any{"id": command["id"], "result": map[string]any{}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	service := NewCDPService(nil)
	service.Endpoint = server.URL + "/json/list"
	service.AllowedOrigin = server.URL
	result, err := service.ReloadAllTargets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetCount != 1 || result.SuccessCount != 1 || result.FailureCount() != 0 {
		t.Fatalf("unexpected reload result: %#v", result)
	}
	commands := []map[string]any{<-commandChannel, <-commandChannel}
	if len(commands) != 2 || commands[0]["method"] != "Runtime.terminateExecution" || commands[1]["method"] != "Page.reload" {
		t.Fatalf("unexpected CDP commands: %#v", commands)
	}
	params, _ := commands[1]["params"].(map[string]any)
	if ignoreCache, _ := params["ignoreCache"].(bool); !ignoreCache {
		t.Fatalf("Page.reload did not bypass the stale UI cache: %#v", commands[1])
	}
}

func TestReloadAllTargetsRejectsMissingCodexPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`[]`))
	}))
	defer server.Close()

	service := NewCDPService(nil)
	service.Endpoint = server.URL
	result, err := service.ReloadAllTargets(context.Background())
	if !errors.Is(err, ErrNoCodexUITargets) {
		t.Fatalf("reload error = %v, want %v", err, ErrNoCodexUITargets)
	}
	if result.TargetCount != 0 || result.SuccessCount != 0 {
		t.Fatalf("unexpected empty reload result: %#v", result)
	}
}
