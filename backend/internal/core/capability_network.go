package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	networkDefaultTimeout = 10 * time.Second
	networkMaximumTimeout = 15 * time.Second
	networkMaximumBody    = 1 << 20
)

type NetworkCapabilityPermissions struct {
	Origins []string `json:"origins"`
	Methods []string `json:"methods,omitempty"`
}

type NetworkCapabilityRequest struct {
	URL          string `json:"url"`
	Method       string `json:"method,omitempty"`
	ResponseType string `json:"responseType,omitempty"`
	TimeoutMS    int    `json:"timeoutMs,omitempty"`
}

type NetworkCapabilityResponse struct {
	Status      int    `json:"status"`
	OK          bool   `json:"ok"`
	FinalURL    string `json:"finalUrl"`
	ContentType string `json:"contentType,omitempty"`
	Body        any    `json:"body,omitempty"`
}

func networkCapabilityDescriptor() CapabilityDescriptor {
	return CapabilityDescriptor{
		DescriptorVersion: 1,
		ID:                NetworkCapabilityID,
		Version:           NetworkCapabilityVersion,
		Summary:           "Performs permission-scoped HTTPS requests in the Go host instead of the Codex renderer.",
		Usage: CapabilityUsageDescriptor{
			UseWhen: []string{
				"A package must call an HTTPS API or resolve a redirect that the Codex page CSP may block.",
				"The request must be constrained to explicit origins and HTTP methods declared before build time.",
			},
			Constraints: []string{
				"Only public HTTPS origins on the default port are allowed; localhost, private, reserved, and link-local addresses are rejected.",
				"Version 1 supports GET and HEAD, follows at most 8 permitted redirects, caps effective timeout at 15000 ms, and caps response bodies at 1 MiB.",
				"Do not use renderer fetch, hidden DOM messages, or localStorage queues as a CSP bypass when this capability is available.",
			},
			ManifestExample: `{
  "network": {
    "version": "^1.0.0",
    "permissions": {
      "origins": ["https://api.example.com"],
      "methods": ["GET"]
    }
  }
}`,
			RuntimeExample: `const network = api.capabilities.require("network");
const response = await network.request({
  url: "https://api.example.com/value",
  method: "GET",
  responseType: "json",
  timeoutMs: 8000
});`,
		},
		Manifest: CapabilityManifestDescriptor{
			RequirementJSONPointer: "/codexTweaks/capabilities/network",
			Fields: []CapabilityFieldDescriptor{
				{
					Path: "version", Type: "string", Format: "semver-requirement", Required: true,
					Description: "Version requirement negotiated independently from codexTweaks.apiVersion.",
				},
				{
					Path: "optional", Type: "boolean", Required: false,
					Description: "When true, an unavailable or incompatible capability is omitted instead of invalidating the package.",
					DefaultJSON: capabilityString("false"),
				},
				{
					Path: "permissions.origins", Type: "array", ItemType: "string", Format: "https-origin", Required: true,
					Description:  "Complete public HTTPS origins permitted for requests and redirects; paths, queries, fragments, credentials, and non-default ports are not allowed.",
					MinimumItems: capabilityInt(1),
				},
				{
					Path: "permissions.methods", Type: "array", ItemType: "string", Format: "http-method", Required: false,
					Description: "Permitted HTTP methods.", Values: []string{http.MethodGet, http.MethodHead},
					DefaultJSON: capabilityString(`["GET"]`),
				},
			},
		},
		Runtime: CapabilityRuntimeDescriptor{
			Scope:          "all-renderers",
			RequiredAccess: `api.capabilities.require("network")`,
			OptionalAccess: `api.capabilities.get("network")`,
			Properties: []CapabilityFieldDescriptor{
				{Path: "id", Type: "string", Required: true, Description: "Stable capability ID.", Values: []string{NetworkCapabilityID}},
				{Path: "version", Type: "string", Format: "semver", Required: true, Description: "Concrete granted version.", Values: []string{NetworkCapabilityVersion}},
			},
			Methods: []CapabilityMethodDescriptor{
				{
					Name: "request", Async: true,
					Signature:   "request(options) => Promise<response>",
					Description: "Performs one authorized HTTPS request and resolves with a normalized response.",
					Inputs: []CapabilityFieldDescriptor{
						{Path: "url", Type: "string", Format: "https-url", Required: true, Description: "Complete URL whose origin must be granted."},
						{Path: "method", Type: "string", Format: "http-method", Required: false, Description: "Granted HTTP method.", Values: []string{http.MethodGet, http.MethodHead}, DefaultJSON: capabilityString(`"GET"`)},
						{Path: "responseType", Type: "string", Required: false, Description: "How to decode the response body.", Values: []string{"json", "text", "none"}, DefaultJSON: capabilityString(`"text"`)},
						{Path: "timeoutMs", Type: "integer", Format: "milliseconds", Required: false, Description: "Request timeout; zero uses 10000 ms and values above 15000 ms are capped.", DefaultJSON: capabilityString("10000"), Minimum: capabilityInt(0), Maximum: capabilityInt(15000)},
					},
					Outputs: []CapabilityFieldDescriptor{
						{Path: "status", Type: "integer", Format: "http-status", Required: true, Description: "HTTP response status code."},
						{Path: "ok", Type: "boolean", Required: true, Description: "True for status codes from 200 through 299."},
						{Path: "finalUrl", Type: "string", Format: "https-url", Required: true, Description: "Final URL after permitted redirects."},
						{Path: "contentType", Type: "string", Required: false, Description: "Response Content-Type header when present."},
						{Path: "body", Type: "any", Format: "json|string|null", Required: false, Description: "Decoded JSON, text, or no value according to responseType and method."},
					},
					Errors: []CapabilityErrorDescriptor{
						{Code: "permission_denied", Description: "The URL origin or HTTP method was not granted."},
						{Code: "invalid_request", Description: "The request shape, responseType, timeout, or URL is invalid."},
						{Code: "timeout", Description: "The request or host bridge timed out."},
						{Code: "network_error", Description: "The request or response body could not be completed."},
						{Code: "response_too_large", Description: "The response body exceeded 1 MiB."},
						{Code: "invalid_response", Description: "A JSON response was not valid JSON."},
						{Code: "busy", Description: "The capability bridge reached its pending-request limit."},
						{Code: "bridge_unavailable", Description: "The renderer has no active host capability bridge."},
					},
				},
			},
		},
	}
}

type networkCapability struct {
	transport http.RoundTripper
	semaphore chan struct{}
}

func newNetworkCapability() *networkCapability {
	resolver := net.DefaultResolver
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialPublicAddress(ctx, resolver, dialer, network, address)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	return &networkCapability{transport: transport, semaphore: make(chan struct{}, 8)}
}

func normalizeNetworkCapabilityPermissions(raw json.RawMessage) (json.RawMessage, error) {
	permissions := NetworkCapabilityPermissions{}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("必须声明 origins")
	}
	if err := decodeCapabilityJSON(raw, &permissions); err != nil {
		return nil, err
	}
	origins := map[string]bool{}
	for _, rawOrigin := range permissions.Origins {
		origin, err := canonicalHTTPSOrigin(rawOrigin)
		if err != nil {
			return nil, err
		}
		origins[origin] = true
	}
	if len(origins) == 0 {
		return nil, errors.New("必须声明至少一个 HTTPS origin")
	}
	methods := map[string]bool{}
	if len(permissions.Methods) == 0 {
		methods[http.MethodGet] = true
	}
	for _, rawMethod := range permissions.Methods {
		method := strings.ToUpper(strings.TrimSpace(rawMethod))
		if method != http.MethodGet && method != http.MethodHead {
			return nil, fmt.Errorf("network@1.0.0 不支持方法 %s", rawMethod)
		}
		methods[method] = true
	}
	permissions.Origins = sortedTrueKeys(origins)
	permissions.Methods = sortedTrueKeys(methods)
	return json.Marshal(permissions)
}

func canonicalHTTPSOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" {
		return "", fmt.Errorf("origin 必须是完整 HTTPS 地址：%s", raw)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("origin 不能包含凭据、路径、查询或片段：%s", raw)
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return "", fmt.Errorf("origin 只允许默认 HTTPS 端口：%s", raw)
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") {
		return "", fmt.Errorf("origin 不能指向本地服务：%s", raw)
	}
	if ip := net.ParseIP(hostname); ip != nil && disallowedNetworkIP(ip) {
		return "", fmt.Errorf("origin 不能指向内网或本地地址：%s", raw)
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return "https://" + host, nil
}

func (n *networkCapability) Invoke(
	ctx context.Context,
	grant GrantedCapability,
	method string,
	parameters json.RawMessage,
) (any, *CapabilityInvocationError) {
	if method != "request" {
		return nil, capabilityFailure("unsupported_method", "Unknown network capability method")
	}
	permissions := NetworkCapabilityPermissions{}
	if err := json.Unmarshal(grant.Permissions, &permissions); err != nil {
		return nil, capabilityFailure("capability_unavailable", "Network permissions are invalid")
	}
	request := NetworkCapabilityRequest{}
	if err := decodeCapabilityJSON(parameters, &request); err != nil {
		return nil, capabilityFailure("invalid_request", "Network request is invalid")
	}
	request.Method = strings.ToUpper(strings.TrimSpace(request.Method))
	if request.Method == "" {
		request.Method = http.MethodGet
	}
	if !containsString(permissions.Methods, request.Method) {
		return nil, capabilityFailure("permission_denied", "HTTP method was not granted")
	}
	request.ResponseType = strings.ToLower(strings.TrimSpace(request.ResponseType))
	if request.ResponseType == "" {
		request.ResponseType = "text"
	}
	if request.ResponseType != "json" && request.ResponseType != "text" && request.ResponseType != "none" {
		return nil, capabilityFailure("invalid_request", "Unsupported responseType")
	}
	if request.TimeoutMS < 0 {
		return nil, capabilityFailure("invalid_request", "timeoutMs cannot be negative")
	}
	if err := validateGrantedNetworkURL(request.URL, permissions); err != nil {
		return nil, capabilityFailure("permission_denied", err.Error())
	}
	timeout := networkDefaultTimeout
	if request.TimeoutMS > 0 {
		timeout = time.Duration(request.TimeoutMS) * time.Millisecond
	}
	if timeout > networkMaximumTimeout {
		timeout = networkMaximumTimeout
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case n.semaphore <- struct{}{}:
		defer func() { <-n.semaphore }()
	case <-requestContext.Done():
		return nil, capabilityFailure("timeout", "Network request timed out")
	}
	httpRequest, err := http.NewRequestWithContext(requestContext, request.Method, request.URL, nil)
	if err != nil {
		return nil, capabilityFailure("invalid_request", "Network URL is invalid")
	}
	httpRequest.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.1")
	httpRequest.Header.Set("User-Agent", "CodexTweaks/1.0")
	client := &http.Client{
		Transport: n.transport,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return errors.New("too many redirects")
			}
			return validateGrantedNetworkURL(next.URL.String(), permissions)
		},
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(requestContext.Err(), context.DeadlineExceeded) {
			return nil, capabilityFailure("timeout", "Network request timed out")
		}
		return nil, capabilityFailure("network_error", "Network request failed")
	}
	defer response.Body.Close()
	result := NetworkCapabilityResponse{
		Status: response.StatusCode, OK: response.StatusCode >= 200 && response.StatusCode < 300,
		FinalURL: response.Request.URL.String(), ContentType: response.Header.Get("Content-Type"),
	}
	if request.ResponseType == "none" || request.Method == http.MethodHead {
		return result, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, networkMaximumBody+1))
	if err != nil {
		return nil, capabilityFailure("network_error", "Network response could not be read")
	}
	if len(body) > networkMaximumBody {
		return nil, capabilityFailure("response_too_large", "Network response exceeded 1 MiB")
	}
	if request.ResponseType == "json" {
		var decoded any
		if len(body) > 0 {
			if err := json.Unmarshal(body, &decoded); err != nil {
				return nil, capabilityFailure("invalid_response", "Network response was not valid JSON")
			}
		}
		result.Body = decoded
	} else {
		result.Body = string(body)
	}
	return result, nil
}

func validateGrantedNetworkURL(raw string, permissions NetworkCapabilityPermissions) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" || parsed.User != nil {
		return errors.New("Only complete HTTPS URLs are allowed")
	}
	origin, err := canonicalHTTPSOrigin((&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String())
	if err != nil || !containsString(permissions.Origins, origin) {
		return errors.New("URL origin was not granted")
	}
	return nil
}

func dialPublicAddress(
	ctx context.Context,
	resolver *net.Resolver,
	dialer *net.Dialer,
	network string,
	address string,
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses := []net.IPAddr{}
	if ip := net.ParseIP(host); ip != nil {
		addresses = append(addresses, net.IPAddr{IP: ip})
	} else {
		addresses, err = resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("hostname did not resolve")
	}
	for _, candidate := range addresses {
		if disallowedNetworkIP(candidate.IP) {
			return nil, errors.New("private or local network address was rejected")
		}
	}
	sort.SliceStable(addresses, func(i, j int) bool {
		return addresses[i].IP.To4() != nil && addresses[j].IP.To4() == nil
	})
	var lastError error
	for _, candidate := range addresses {
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastError = dialErr
	}
	return nil, lastError
}

func disallowedNetworkIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	for _, prefix := range networkReservedPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

var networkReservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}
