package core

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestAvailableCapabilitiesAreIndependentlyVersioned(t *testing.T) {
	got := AvailableCapabilities()
	if len(got) != 2 || got[0].ID != NetworkCapabilityID || got[1].ID != UISettingsSectionCapabilityID {
		t.Fatalf("available capabilities are not stable and sorted: %#v", got)
	}
	for _, descriptor := range got {
		if descriptor.DescriptorVersion != 1 || descriptor.Version == "" || descriptor.Summary == "" ||
			len(descriptor.Usage.UseWhen) == 0 || len(descriptor.Usage.Constraints) == 0 ||
			descriptor.Usage.ManifestExample == "" || descriptor.Usage.RuntimeExample == "" ||
			descriptor.Manifest.RequirementJSONPointer == "" || len(descriptor.Manifest.Fields) == 0 ||
			descriptor.Runtime.RequiredAccess == "" || descriptor.Runtime.OptionalAccess == "" ||
			len(descriptor.Runtime.Methods) == 0 {
			t.Fatalf("capability descriptor is incomplete: %#v", descriptor)
		}
		for _, field := range descriptor.Manifest.Fields {
			if field.DefaultJSON == nil {
				continue
			}
			var value any
			if err := json.Unmarshal([]byte(*field.DefaultJSON), &value); err != nil {
				t.Fatalf("%s field %s has invalid defaultJSON %q: %v", descriptor.ID, field.Path, *field.DefaultJSON, err)
			}
		}
		for _, method := range descriptor.Runtime.Methods {
			if method.Signature == "" || method.Description == "" || method.Inputs == nil ||
				method.Outputs == nil || method.Errors == nil {
				t.Fatalf("%s method descriptor is incomplete: %#v", descriptor.ID, method)
			}
		}
	}
	if got[0].Version != NetworkCapabilityVersion ||
		got[0].Manifest.RequirementJSONPointer != "/codexTweaks/capabilities/network" ||
		got[0].Runtime.Methods[0].Name != "request" {
		t.Fatalf("network descriptor is inconsistent: %#v", got[0])
	}
	if got[1].Version != UISettingsSectionCapabilityVersion ||
		got[1].Manifest.RequirementJSONPointer != "/codexTweaks/capabilities/ui.settings-section" ||
		got[1].Runtime.Methods[1].Name != "register" {
		t.Fatalf("settings descriptor is inconsistent: %#v", got[1])
	}
	encoded, err := json.Marshal(got)
	if err != nil || !strings.Contains(string(encoded), `"manifestExample"`) ||
		!strings.Contains(string(encoded), `"runtimeExample"`) ||
		!strings.Contains(string(encoded), `"requiredAccess"`) {
		t.Fatalf("machine-readable capability catalog is incomplete: %s, %v", encoded, err)
	}

	got[0].Usage.UseWhen[0] = "mutated"
	got[0].Manifest.Fields[0].Description = "mutated"
	got[0].Runtime.Methods[0].Errors[0].Description = "mutated"
	fresh := AvailableCapabilities()
	if fresh[0].Usage.UseWhen[0] == "mutated" || fresh[0].Manifest.Fields[0].Description == "mutated" ||
		fresh[0].Runtime.Methods[0].Errors[0].Description == "mutated" {
		t.Fatal("available capability descriptors share mutable catalog storage")
	}
}

func TestResolveAndBindPackageCapabilities(t *testing.T) {
	requirements := map[string]CapabilityRequirement{
		NetworkCapabilityID: {
			Version: "^1.0.0",
			Permissions: json.RawMessage(`{
              "origins":["https://B.example/", "https://a.example"],
              "methods":["head", "GET", "GET"]
            }`),
		},
		UISettingsSectionCapabilityID: {
			Version: "1",
			Permissions: json.RawMessage(`{
              "sections":[{"id":"wallpaper","title":"随机背景","after":"personalization"}]
            }`),
		},
		"future.optional": {Version: "*", Optional: true},
	}
	grants, err := resolvePackageCapabilities("Codex Random Background", requirements)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 2 {
		t.Fatalf("grants = %#v", grants)
	}
	network := NetworkCapabilityPermissions{}
	if err := json.Unmarshal(grants[NetworkCapabilityID].Permissions, &network); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(network.Origins, []string{"https://a.example", "https://b.example"}) ||
		!reflect.DeepEqual(network.Methods, []string{http.MethodGet, http.MethodHead}) {
		t.Fatalf("normalized network permissions = %#v", network)
	}
	settings := UISettingsSectionPermissions{}
	if err := json.Unmarshal(grants[UISettingsSectionCapabilityID].Permissions, &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Sections) != 1 || settings.Sections[0].Group != "personal" ||
		settings.Sections[0].Icon != "personalization" ||
		!strings.HasPrefix(settings.Sections[0].Slug, "codex-tweaks-codex-random-background-wallpaper-") {
		t.Fatalf("bound settings permissions = %#v", settings)
	}
}

func TestCapabilityRequirementsRejectUnavailableVersionsAndInvalidPermissions(t *testing.T) {
	tests := []map[string]CapabilityRequirement{
		{"unknown": {Version: "1.0.0"}},
		{NetworkCapabilityID: {Version: "^2.0.0", Permissions: json.RawMessage(`{"origins":["https://example.com"]}`)}},
		{NetworkCapabilityID: {Version: "1.0.0", Permissions: json.RawMessage(`{"origins":["http://example.com"]}`)}},
		{NetworkCapabilityID: {Version: "1.0.0", Permissions: json.RawMessage(`{"origins":["https://example.com"]} {}`)}},
		{UISettingsSectionCapabilityID: {Version: "1.0.0", Permissions: json.RawMessage(`{"sections":[{"id":"bad id","title":"Bad"}]}`)}},
	}
	for _, requirements := range tests {
		if _, err := ResolveCapabilityRequirements(requirements); err == nil {
			t.Fatalf("expected requirements to fail: %#v", requirements)
		}
	}
}

func TestNetworkCapabilityRejectsReservedAddressesAndInvalidTimeout(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254",
		"192.0.2.1", "198.18.0.1", "203.0.113.1", "2001:db8::1", "::1",
	} {
		if !disallowedNetworkIP(net.ParseIP(raw)) {
			t.Fatalf("reserved address was allowed: %s", raw)
		}
	}
	for _, raw := range []string{"1.1.1.1", "2606:4700:4700::1111"} {
		if disallowedNetworkIP(net.ParseIP(raw)) {
			t.Fatalf("public address was rejected: %s", raw)
		}
	}

	permissions, err := normalizeNetworkCapabilityPermissions(
		json.RawMessage(`{"origins":["https://example.com"]}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	network := &networkCapability{
		semaphore: make(chan struct{}, 1),
		transport: capabilityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			t.Fatal("negative timeout should be rejected before transport")
			return nil, nil
		}),
	}
	_, invocationError := network.Invoke(
		context.Background(),
		GrantedCapability{Version: NetworkCapabilityVersion, Permissions: permissions},
		"request",
		json.RawMessage(`{"url":"https://example.com","timeoutMs":-1}`),
	)
	if invocationError == nil || invocationError.Code != "invalid_request" {
		t.Fatalf("negative timeout was not rejected: %#v", invocationError)
	}
}

func TestNetworkCapabilityBrokersGrantedHTTPSRequests(t *testing.T) {
	permissions, err := normalizeNetworkCapabilityPermissions(
		json.RawMessage(`{"origins":["https://example.com"],"methods":["GET"]}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	requestCount := 0
	network := &networkCapability{
		semaphore: make(chan struct{}, 1),
		transport: capabilityRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestCount++
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"image":"https://cdn.example/image.webp"}`)),
				Request:    request,
			}, nil
		}),
	}
	grant := GrantedCapability{Version: NetworkCapabilityVersion, Permissions: permissions}
	result, invocationError := network.Invoke(
		context.Background(), grant, "request",
		json.RawMessage(`{"url":"https://example.com/random","responseType":"json","timeoutMs":1000}`),
	)
	if invocationError != nil {
		t.Fatalf("network invocation failed: %#v", invocationError)
	}
	response, ok := result.(NetworkCapabilityResponse)
	if !ok || !response.OK || response.Status != http.StatusOK || requestCount != 1 {
		t.Fatalf("network response = %#v, requests = %d", result, requestCount)
	}
	if _, invocationError := network.Invoke(
		context.Background(), grant, "request",
		json.RawMessage(`{"url":"https://ungranted.example/random"}`),
	); invocationError == nil || invocationError.Code != "permission_denied" || requestCount != 1 {
		t.Fatalf("ungranted request was not rejected: %#v", invocationError)
	}
}

type capabilityRoundTripFunc func(*http.Request) (*http.Response, error)

func (function capabilityRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
