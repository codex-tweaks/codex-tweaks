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
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	rendererBridgeBindingName       = "__codexTweaksHostBridge"
	rendererBridgeVersion           = 1
	rendererBridgeMaximumPayload    = 4 << 20
	rendererBridgeMaximumConcurrent = 32
	rendererBridgeMaximumResponses  = 8
)

var errRendererBridgeSessionClosed = errors.New("CDP renderer bridge session closed")

type cdpRendererEnvelope struct {
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

type rendererBindingEvent struct {
	Name               string `json:"name"`
	Payload            string `json:"payload"`
	ExecutionContextID int    `json:"executionContextId"`
}

type cdpScriptParsedEvent struct {
	ScriptID string `json:"scriptId"`
	URL      string `json:"url"`
}

type rendererBridgeRequest struct {
	Version    int             `json:"v"`
	ID         string          `json:"id"`
	Token      string          `json:"token"`
	Method     string          `json:"method"`
	Parameters json.RawMessage `json:"parameters"`
}

type rendererBridgeResponse struct {
	ID     string               `json:"id"`
	OK     bool                 `json:"ok"`
	Result json.RawMessage      `json:"result,omitempty"`
	Error  *NodeInvocationError `json:"error,omitempty"`
}

type rendererAuthorization struct {
	packageID string
}

type rendererBridgeSession struct {
	id          string
	debuggerURL string
	connection  *websocket.Conn
	logger      *Logger
	secret      string
	invoker     NodeInvoker
	invokerMu   sync.RWMutex

	writeMu sync.Mutex
	stateMu sync.Mutex
	nextID  int
	pending map[int]chan cdpCallResult
	done    chan struct{}
	closed  bool

	authorizationMu sync.RWMutex
	authorizations  map[string]rendererAuthorization
	requestSlots    chan struct{}
	responseSlots   chan struct{}
	closeOnce       sync.Once

	scriptMu              sync.RWMutex
	scripts               map[string]string
	executionGeneration   atomic.Uint64
	debuggerEnabled       bool
	settingsAppModuleURL  string
	settingsVisibilityURL string
	adapterMu             sync.Mutex
	settingsAdapterCached bool
	settingsAdapterGen    uint64
}

func openRendererBridgeSession(
	ctx context.Context,
	dialer *websocket.Dialer,
	allowedOrigin string,
	target CDPTarget,
	invoker NodeInvoker,
	logger *Logger,
) (*rendererBridgeSession, error) {
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
	session := &rendererBridgeSession{
		id: id, debuggerURL: *target.WebSocketDebuggerURL, connection: connection,
		invoker: invoker, logger: logger, secret: secret, nextID: 1,
		pending: map[int]chan cdpCallResult{}, done: make(chan struct{}),
		authorizations: map[string]rendererAuthorization{},
		requestSlots:   make(chan struct{}, rendererBridgeMaximumConcurrent),
		responseSlots:  make(chan struct{}, rendererBridgeMaximumResponses),
		scripts:        map[string]string{},
	}
	go session.readLoop()
	setupContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := session.call(setupContext, "Runtime.enable", map[string]any{}); err != nil {
		session.Close()
		return nil, err
	}
	if _, err := session.call(setupContext, "Runtime.addBinding", map[string]any{"name": rendererBridgeBindingName}); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func (s *rendererBridgeSession) Closed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *rendererBridgeSession) Close() { s.fail(errRendererBridgeSessionClosed) }

func (s *rendererBridgeSession) fail(sessionError error) {
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

func (s *rendererBridgeSession) call(ctx context.Context, method string, parameters any) (json.RawMessage, error) {
	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		return nil, errRendererBridgeSessionClosed
	}
	commandID := s.nextID
	s.nextID++
	waiter := make(chan cdpCallResult, 1)
	s.pending[commandID] = waiter
	s.stateMu.Unlock()

	s.writeMu.Lock()
	_ = s.connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
	err := s.connection.WriteJSON(map[string]any{"id": commandID, "method": method, "params": parameters})
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
		return nil, errRendererBridgeSessionClosed
	}
}

func (s *rendererBridgeSession) removePending(commandID int) {
	s.stateMu.Lock()
	delete(s.pending, commandID)
	s.stateMu.Unlock()
}

func (s *rendererBridgeSession) readLoop() {
	for {
		_, message, err := s.connection.ReadMessage()
		if err != nil {
			s.fail(err)
			return
		}
		envelope := cdpRendererEnvelope{}
		if json.Unmarshal(message, &envelope) != nil {
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
		switch envelope.Method {
		case "Runtime.bindingCalled":
			s.dispatchBindingEvent(envelope.Params)
		case "Debugger.scriptParsed":
			event := cdpScriptParsedEvent{}
			if json.Unmarshal(envelope.Params, &event) == nil && event.ScriptID != "" && event.URL != "" {
				s.scriptMu.Lock()
				s.scripts[event.URL] = event.ScriptID
				s.scriptMu.Unlock()
			}
		case "Runtime.executionContextsCleared":
			s.executionGeneration.Add(1)
			s.scriptMu.Lock()
			clear(s.scripts)
			s.scriptMu.Unlock()
		}
	}
}

func (s *rendererBridgeSession) authorize(payload Payload) map[string]string {
	authorizations := map[string]rendererAuthorization{}
	tokens := map[string]string{}
	for _, pkg := range payload.Packages {
		if pkg.Node == nil {
			continue
		}
		mac := hmac.New(sha256.New, []byte(s.secret))
		_, _ = mac.Write([]byte(strings.Join([]string{payload.Version, pkg.ID, pkg.Node.AuthorizationID}, "\x00")))
		token := hex.EncodeToString(mac.Sum(nil))
		tokens[pkg.ID] = token
		authorizations[token] = rendererAuthorization{packageID: pkg.ID}
	}
	s.authorizationMu.Lock()
	s.authorizations = authorizations
	s.authorizationMu.Unlock()
	return tokens
}

func (s *rendererBridgeSession) dispatchBindingEvent(raw json.RawMessage) {
	event := rendererBindingEvent{}
	if json.Unmarshal(raw, &event) != nil || event.Name != rendererBridgeBindingName || event.ExecutionContextID <= 0 {
		return
	}
	if len(event.Payload) == 0 || len(event.Payload) > rendererBridgeMaximumPayload {
		return
	}
	request := rendererBridgeRequest{}
	if json.Unmarshal([]byte(event.Payload), &request) != nil {
		return
	}
	select {
	case s.requestSlots <- struct{}{}:
		go func() {
			defer func() { <-s.requestSlots }()
			s.handleBridgeRequest(event.ExecutionContextID, request)
		}()
	default:
		select {
		case s.responseSlots <- struct{}{}:
			go func() {
				defer func() { <-s.responseSlots }()
				s.sendBridgeResponse(event.ExecutionContextID, rendererBridgeResponse{
					ID: request.ID, Error: &NodeInvocationError{Code: "busy", Message: "Renderer bridge is busy"},
				})
			}()
		default:
		}
	}
}

func (s *rendererBridgeSession) handleBridgeRequest(executionContextID int, request rendererBridgeRequest) {
	if request.Version != rendererBridgeVersion || request.ID == "" || len(request.ID) > 200 || request.Token == "" || strings.TrimSpace(request.Method) == "" {
		s.sendBridgeResponse(executionContextID, rendererBridgeResponse{
			ID: request.ID, Error: &NodeInvocationError{Code: "invalid_request", Message: "Renderer bridge request is invalid"},
		})
		return
	}
	s.authorizationMu.RLock()
	authorization, allowed := s.authorizations[request.Token]
	s.authorizationMu.RUnlock()
	if !allowed {
		s.sendBridgeResponse(executionContextID, rendererBridgeResponse{
			ID: request.ID, Error: &NodeInvocationError{Code: "permission_denied", Message: "Node token is invalid"},
		})
		return
	}
	s.invokerMu.RLock()
	invoker := s.invoker
	s.invokerMu.RUnlock()
	if invoker == nil {
		s.sendBridgeResponse(executionContextID, rendererBridgeResponse{
			ID: request.ID, Error: &NodeInvocationError{Code: "node_unavailable", Message: "Node runtime is unavailable"},
		})
		return
	}
	requestContext, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, invocationError := invoker.InvokeNode(requestContext, authorization.packageID, request.Method, request.Parameters)
	s.sendBridgeResponse(executionContextID, rendererBridgeResponse{
		ID: request.ID, OK: invocationError == nil, Result: result, Error: invocationError,
	})
}

func (s *rendererBridgeSession) sendBridgeResponse(executionContextID int, response rendererBridgeResponse) {
	expression := fmt.Sprintf(`(() => {
  try {
    return globalThis["__CODEX_TWEAKS__"]?.settleNodeInvocation(%s) ?? false;
  } catch (_) {
    return false;
  }
})()`, JSONLiteral(response))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "contextId": executionContextID,
		"returnByValue": true, "awaitPromise": false, "userGesture": false,
	}); err != nil && !errors.Is(err, errRendererBridgeSessionClosed) && !errors.Is(err, context.Canceled) && s.logger != nil {
		s.logger.Error("Node 调用结果无法返回 Codex 页面：" + err.Error())
	}
}

func (s *rendererBridgeSession) sendNodeEvent(event NodeRuntimeEvent) {
	expression := fmt.Sprintf(`(() => {
  try {
    return globalThis["__CODEX_TWEAKS__"]?.emitNodeEvent(%s, %s, %s) ?? false;
  } catch (_) {
    return false;
  }
})()`, JSONLiteral(event.PackageID), JSONLiteral(event.Name), string(event.Payload))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "returnByValue": true, "awaitPromise": false, "userGesture": false,
	})
}

func payloadNeedsRendererBridge(payload Payload) bool {
	for _, pkg := range payload.Packages {
		if pkg.Node != nil || pkg.UI.SettingsSections != nil {
			return true
		}
	}
	return false
}

func (s *CDPService) rendererBridgeForTargetLocked(
	ctx context.Context,
	target CDPTarget,
	payload Payload,
) (string, map[string]string, *SettingsAdapterConfiguration, error) {
	if !payloadNeedsRendererBridge(payload) {
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
		created, err := openRendererBridgeSession(ctx, s.dialer, s.AllowedOrigin, target, s.nodeInvoker, s.logger)
		if err != nil {
			return "", nil, nil, err
		}
		s.sessions[target.ID] = created
		existing = created
	} else {
		existing.invokerMu.Lock()
		existing.invoker = s.nodeInvoker
		existing.invokerMu.Unlock()
	}
	adapterContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	settingsAdapter, err := existing.ensureSettingsAdapter(adapterContext, payload)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("ui.settingsSections@1 无法适配当前 Codex：" + err.Error())
		}
		settingsAdapter = nil
	}
	return existing.id, existing.authorize(payload), settingsAdapter, nil
}

func (s *CDPService) reconcileRendererSessionsLocked(targets []CDPTarget) {
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

func (s *CDPService) closeAllRendererSessionsLocked() {
	for targetID, session := range s.sessions {
		session.Close()
		delete(s.sessions, targetID)
	}
}

func (s *CDPService) SetNodeInvoker(invoker NodeInvoker) {
	s.mu.Lock()
	s.nodeInvoker = invoker
	for _, session := range s.sessions {
		session.invokerMu.Lock()
		session.invoker = invoker
		session.invokerMu.Unlock()
	}
	s.mu.Unlock()
}

func (s *CDPService) EmitNodeEvent(event NodeRuntimeEvent) {
	if len(event.Payload) == 0 {
		event.Payload = json.RawMessage("null")
	}
	s.mu.Lock()
	sessions := make([]*rendererBridgeSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()
	for _, session := range sessions {
		go session.sendNodeEvent(event)
	}
}
