package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	capabilityBindingName       = "__codexTweaksHostCapability"
	capabilityBridgeVersion     = 1
	capabilityMaximumPayload    = 64 << 10
	capabilityMaximumConcurrent = 32
	capabilityMaximumResponses  = 8
)

var errCapabilitySessionClosed = errors.New("CDP capability session closed")

type cdpCapabilityEnvelope struct {
	ID     *int             `json:"id,omitempty"`
	Method string           `json:"method,omitempty"`
	Params json.RawMessage  `json:"params,omitempty"`
	Result json.RawMessage  `json:"result,omitempty"`
	Error  *cdpCommandError `json:"error,omitempty"`
}

type cdpCommandError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cdpCallResult struct {
	result json.RawMessage
	err    error
}

type capabilityBindingEvent struct {
	Name               string `json:"name"`
	Payload            string `json:"payload"`
	ExecutionContextID int    `json:"executionContextId"`
}

type cdpScriptParsedEvent struct {
	ScriptID string `json:"scriptId"`
	URL      string `json:"url"`
}

type capabilityBridgeRequest struct {
	Version    int             `json:"v"`
	ID         string          `json:"id"`
	Token      string          `json:"token"`
	Capability string          `json:"capability"`
	Method     string          `json:"method"`
	Parameters json.RawMessage `json:"parameters"`
}

type capabilityBridgeResponse struct {
	ID     string                     `json:"id"`
	OK     bool                       `json:"ok"`
	Result any                        `json:"result,omitempty"`
	Error  *CapabilityInvocationError `json:"error,omitempty"`
}

type capabilityAuthorization struct {
	packageID string
	grants    map[string]GrantedCapability
}

type capabilitySession struct {
	id          string
	debuggerURL string
	connection  *websocket.Conn
	broker      *CapabilityBroker
	logger      *Logger
	secret      string

	writeMu sync.Mutex
	stateMu sync.Mutex
	nextID  int
	pending map[int]chan cdpCallResult
	done    chan struct{}
	closed  bool

	authorizationMu sync.RWMutex
	authorizations  map[string]capabilityAuthorization
	requestSlots    chan struct{}
	responseSlots   chan struct{}
	closeOnce       sync.Once

	scriptMu              sync.RWMutex
	scripts               map[string]string
	debuggerEnabled       bool
	settingsScriptID      string
	settingsAppModuleURL  string
	settingsVisibilityURL string
	adapterMu             sync.Mutex
}

func openCapabilitySession(
	ctx context.Context,
	dialer *websocket.Dialer,
	allowedOrigin string,
	target CDPTarget,
	broker *CapabilityBroker,
	logger *Logger,
) (*capabilitySession, error) {
	header := http.Header{}
	header.Set("Origin", allowedOrigin)
	connection, _, err := dialer.DialContext(ctx, *target.WebSocketDebuggerURL, header)
	if err != nil {
		return nil, err
	}
	id, err := randomHex(12)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	secret, err := randomHex(32)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	session := &capabilitySession{
		id: id, debuggerURL: *target.WebSocketDebuggerURL, connection: connection,
		broker: broker, logger: logger, secret: secret, nextID: 1,
		pending: map[int]chan cdpCallResult{}, done: make(chan struct{}),
		authorizations: map[string]capabilityAuthorization{},
		requestSlots:   make(chan struct{}, capabilityMaximumConcurrent),
		responseSlots:  make(chan struct{}, capabilityMaximumResponses),
		scripts:        map[string]string{},
	}
	go session.readLoop()
	setupContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := session.call(setupContext, "Runtime.enable", map[string]any{}); err != nil {
		session.Close()
		return nil, err
	}
	if _, err := session.call(setupContext, "Runtime.addBinding", map[string]any{"name": capabilityBindingName}); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func (s *capabilitySession) Closed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *capabilitySession) Close() {
	s.fail(errCapabilitySessionClosed)
}

func (s *capabilitySession) fail(sessionError error) {
	s.closeOnce.Do(func() {
		s.stateMu.Lock()
		s.closed = true
		pending := s.pending
		s.pending = map[int]chan cdpCallResult{}
		s.stateMu.Unlock()
		for _, waiter := range pending {
			waiter <- cdpCallResult{err: sessionError}
		}
		close(s.done)
		_ = s.connection.Close()
	})
}

func (s *capabilitySession) call(ctx context.Context, method string, parameters any) (json.RawMessage, error) {
	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		return nil, errCapabilitySessionClosed
	}
	commandID := s.nextID
	s.nextID++
	waiter := make(chan cdpCallResult, 1)
	s.pending[commandID] = waiter
	s.stateMu.Unlock()

	command := map[string]any{"id": commandID, "method": method, "params": parameters}
	s.writeMu.Lock()
	_ = s.connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
	err := s.connection.WriteJSON(command)
	s.writeMu.Unlock()
	if err != nil {
		s.removePending(commandID)
		s.fail(err)
		return nil, err
	}

	select {
	case response := <-waiter:
		return response.result, response.err
	case <-ctx.Done():
		s.removePending(commandID)
		return nil, ctx.Err()
	case <-s.done:
		s.removePending(commandID)
		return nil, errCapabilitySessionClosed
	}
}

func (s *capabilitySession) removePending(commandID int) {
	s.stateMu.Lock()
	delete(s.pending, commandID)
	s.stateMu.Unlock()
}

func (s *capabilitySession) readLoop() {
	for {
		_, message, err := s.connection.ReadMessage()
		if err != nil {
			s.fail(err)
			return
		}
		envelope := cdpCapabilityEnvelope{}
		if err := json.Unmarshal(message, &envelope); err != nil {
			continue
		}
		if envelope.ID != nil {
			s.stateMu.Lock()
			waiter := s.pending[*envelope.ID]
			delete(s.pending, *envelope.ID)
			s.stateMu.Unlock()
			if waiter != nil {
				if envelope.Error != nil {
					waiter <- cdpCallResult{err: fmt.Errorf("CDP 拒绝执行：%s", envelope.Error.Message)}
				} else {
					waiter <- cdpCallResult{result: envelope.Result}
				}
			}
			continue
		}
		if envelope.Method == "Runtime.bindingCalled" {
			s.dispatchBindingEvent(envelope.Params)
		} else if envelope.Method == "Debugger.scriptParsed" {
			event := cdpScriptParsedEvent{}
			if json.Unmarshal(envelope.Params, &event) == nil && event.ScriptID != "" && event.URL != "" {
				s.scriptMu.Lock()
				s.scripts[event.URL] = event.ScriptID
				s.scriptMu.Unlock()
			}
		}
	}
}

func (s *capabilitySession) authorize(payload Payload) map[string]string {
	authorizations := map[string]capabilityAuthorization{}
	tokens := map[string]string{}
	for _, pkg := range payload.Packages {
		if len(pkg.Capabilities) == 0 {
			continue
		}
		capabilities, _ := json.Marshal(pkg.Capabilities)
		mac := hmac.New(sha256.New, []byte(s.secret))
		_, _ = mac.Write([]byte(strings.Join([]string{payload.Version, pkg.ID, string(capabilities)}, "\x00")))
		token := hex.EncodeToString(mac.Sum(nil))
		tokens[pkg.ID] = token
		authorizations[token] = capabilityAuthorization{
			packageID: pkg.ID,
			grants:    cloneGrantedCapabilities(pkg.Capabilities),
		}
	}
	s.authorizationMu.Lock()
	s.authorizations = authorizations
	s.authorizationMu.Unlock()
	return tokens
}

func cloneGrantedCapabilities(source map[string]GrantedCapability) map[string]GrantedCapability {
	result := make(map[string]GrantedCapability, len(source))
	for id, grant := range source {
		grant.Permissions = append(json.RawMessage(nil), grant.Permissions...)
		result[id] = grant
	}
	return result
}

func (s *capabilitySession) dispatchBindingEvent(raw json.RawMessage) {
	event := capabilityBindingEvent{}
	if err := json.Unmarshal(raw, &event); err != nil || event.Name != capabilityBindingName || event.ExecutionContextID <= 0 {
		return
	}
	if len(event.Payload) == 0 || len(event.Payload) > capabilityMaximumPayload {
		return
	}
	request := capabilityBridgeRequest{}
	if err := json.Unmarshal([]byte(event.Payload), &request); err != nil {
		return
	}
	select {
	case s.requestSlots <- struct{}{}:
		go func() {
			defer func() { <-s.requestSlots }()
			s.handleCapabilityRequest(event.ExecutionContextID, request)
		}()
	default:
		select {
		case s.responseSlots <- struct{}{}:
			go func() {
				defer func() { <-s.responseSlots }()
				s.sendCapabilityResponse(event.ExecutionContextID, capabilityBridgeResponse{
					ID: request.ID, OK: false,
					Error: capabilityFailure("busy", "Capability bridge is busy"),
				})
			}()
		default:
		}
	}
}

func (s *capabilitySession) handleCapabilityRequest(executionContextID int, request capabilityBridgeRequest) {
	if request.Version != capabilityBridgeVersion || request.ID == "" || len(request.ID) > 200 ||
		request.Token == "" || request.Capability == "" || request.Method == "" {
		s.sendCapabilityResponse(executionContextID, capabilityBridgeResponse{
			ID: request.ID, OK: false,
			Error: capabilityFailure("invalid_request", "Capability request is invalid"),
		})
		return
	}
	s.authorizationMu.RLock()
	authorization, allowed := s.authorizations[request.Token]
	s.authorizationMu.RUnlock()
	if !allowed {
		s.sendCapabilityResponse(executionContextID, capabilityBridgeResponse{
			ID: request.ID, OK: false,
			Error: capabilityFailure("permission_denied", "Capability token is invalid"),
		})
		return
	}
	requestContext, cancel := context.WithTimeout(context.Background(), networkMaximumTimeout+time.Second)
	defer cancel()
	result, invocationError := s.broker.Invoke(
		requestContext, authorization.grants, request.Capability, request.Method, request.Parameters,
	)
	response := capabilityBridgeResponse{ID: request.ID, OK: invocationError == nil, Result: result, Error: invocationError}
	s.sendCapabilityResponse(executionContextID, response)
}

func (s *capabilitySession) sendCapabilityResponse(executionContextID int, response capabilityBridgeResponse) {
	expression := fmt.Sprintf(`(() => {
  try {
    return globalThis["__CODEX_TWEAKS__"]?.settleCapability(%s) ?? false;
  } catch (_) {
    return false;
  }
})()`, JSONLiteral(response))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "contextId": executionContextID,
		"returnByValue": true, "awaitPromise": false, "userGesture": false,
	}); err != nil && !errors.Is(err, errCapabilitySessionClosed) && !errors.Is(err, context.Canceled) {
		if s.logger != nil {
			s.logger.Error("能力调用结果无法返回 Codex 页面：" + err.Error())
		}
	}
}

func payloadUsesCapabilities(payload Payload) bool {
	for _, pkg := range payload.Packages {
		if len(pkg.Capabilities) > 0 {
			return true
		}
	}
	return false
}

func (s *CDPService) capabilityBridgeForTargetLocked(
	ctx context.Context,
	target CDPTarget,
	payload Payload,
) (string, map[string]string, *SettingsAdapterConfiguration, error) {
	if !payloadUsesCapabilities(payload) {
		if existing := s.sessions[target.ID]; existing != nil {
			existing.Close()
			delete(s.sessions, target.ID)
		}
		return "", map[string]string{}, nil, nil
	}
	existing := s.sessions[target.ID]
	if existing != nil && (existing.Closed() || existing.debuggerURL != *target.WebSocketDebuggerURL) {
		existing.Close()
		delete(s.sessions, target.ID)
		existing = nil
	}
	if existing == nil {
		created, err := openCapabilitySession(ctx, s.dialer, s.AllowedOrigin, target, s.broker, s.logger)
		if err != nil {
			return "", nil, nil, err
		}
		s.sessions[target.ID] = created
		existing = created
	}
	adapterContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	settingsAdapter, err := existing.ensureSettingsAdapter(adapterContext, payload)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("ui.settings-section@1.0.0 无法适配当前 Codex：" + err.Error())
		}
		settingsAdapter = nil
	}
	return existing.id, existing.authorize(payload), settingsAdapter, nil
}

func (s *CDPService) reconcileCapabilitySessionsLocked(targets []CDPTarget) {
	current := map[string]string{}
	for _, target := range targets {
		current[target.ID] = *target.WebSocketDebuggerURL
	}
	for targetID, session := range s.sessions {
		debuggerURL, present := current[targetID]
		if !present || debuggerURL != session.debuggerURL || session.Closed() {
			session.Close()
			delete(s.sessions, targetID)
		}
	}
}

func (s *CDPService) closeAllCapabilitySessionsLocked() {
	for targetID, session := range s.sessions {
		session.Close()
		delete(s.sessions, targetID)
	}
}
