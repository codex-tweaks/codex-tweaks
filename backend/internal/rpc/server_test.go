package rpc

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codex-tweaks/codex-tweaks/backend/internal/core"
)

func TestServerPingAndPreInitializationGuard(t *testing.T) {
	input := strings.NewReader(
		"{\"id\":1,\"method\":\"ping\"}\n" +
			"{\"id\":2,\"method\":\"getState\"}\n",
	)
	var output bytes.Buffer
	if err := NewServer(input, &output).Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}

	lines := responseLines(t, output.String())
	if got := lines[0].Result["backend"]; got != "go" {
		t.Fatalf("backend = %#v", got)
	}
	if got := lines[0].Result["protocolVersion"]; got != float64(core.ProtocolVersion) {
		t.Fatalf("protocolVersion = %#v", got)
	}
	if lines[1].Error == nil || !strings.Contains(lines[1].Error.Message, "initialize") {
		t.Fatalf("expected initialization error, got %#v", lines[1])
	}
}

func TestServerInitializesControllerWithoutBackgroundSideEffects(t *testing.T) {
	root := t.TempDir()
	params := `{
      "applicationSupportDirectory": ` + quoted(filepath.Join(root, "support")) + `,
      "cacheDirectory": ` + quoted(filepath.Join(root, "cache")) + `,
      "currentVersion": "0.1.0",
      "buildNumber": "1"
    }`
	initializeRequest, err := json.Marshal(request{
		ID: 1, Method: "initialize", Params: json.RawMessage(params),
	})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader(
		string(initializeRequest) + "\n" +
			"{\"id\":2,\"method\":\"getState\"}\n" +
			"{\"id\":3,\"method\":\"shutdown\"}\n",
	)
	var output bytes.Buffer
	server := NewServerWithDependencies(
		input,
		&output,
		core.ControllerDependencies{DisableBackground: true},
	)
	if err := server.Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}

	var initialize struct {
		ID     int `json:"id"`
		Result struct {
			ProtocolVersion int `json:"protocolVersion"`
		} `json:"result"`
	}
	foundInitialize := false
	var state struct {
		ID     int `json:"id"`
		Result struct {
			ProtocolVersion           int  `json:"protocolVersion"`
			DeveloperAllowUnknownNode bool `json:"developerAllowUnknownNode"`
		} `json:"result"`
	}
	foundState := false
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var header struct {
			ID *int `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &header); err != nil {
			t.Fatalf("decode header: %v", err)
		}
		if header.ID != nil && *header.ID == 1 {
			if err := json.Unmarshal([]byte(line), &initialize); err != nil {
				t.Fatalf("decode initialize: %v", err)
			}
			foundInitialize = true
		}
		if header.ID != nil && *header.ID == 2 {
			if err := json.Unmarshal([]byte(line), &state); err != nil {
				t.Fatalf("decode getState: %v", err)
			}
			foundState = true
		}
	}
	if !foundInitialize || initialize.Result.ProtocolVersion != core.ProtocolVersion {
		t.Fatalf("initialize response not found in %q", output.String())
	}
	if !foundState || state.Result.ProtocolVersion != core.ProtocolVersion || state.Result.DeveloperAllowUnknownNode {
		t.Fatalf("getState did not expose the v8 non-persistent Node default: %q", output.String())
	}
}

type decodedResponse struct {
	ID     int                    `json:"id"`
	Result map[string]interface{} `json:"result"`
	Error  *rpcError              `json:"error"`
}

func responseLines(t *testing.T, output string) []decodedResponse {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	result := make([]decodedResponse, 0, len(lines))
	for _, line := range lines {
		var response decodedResponse
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		result = append(result, response)
	}
	return result
}

func quoted(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
