package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	ErrCDPEndpointUnavailable = errors.New("Codex 未开启本地 CDP 端口")
	ErrNoCodexUITargets       = errors.New("没有发现可重启的 Codex 界面")
)

type CDPTarget struct {
	ID                   string  `json:"id"`
	Type                 string  `json:"type"`
	Title                string  `json:"title"`
	URL                  string  `json:"url"`
	WebSocketDebuggerURL *string `json:"webSocketDebuggerUrl,omitempty"`
}

func (t CDPTarget) Injectable() bool {
	if t.Type != "page" || !strings.HasPrefix(strings.ToLower(t.URL), "app://") || t.WebSocketDebuggerURL == nil {
		return false
	}
	parsed, err := url.Parse(*t.WebSocketDebuggerURL)
	return err == nil && (parsed.Scheme == "ws" || parsed.Scheme == "wss") && parsed.Host != ""
}

func InjectableTargets(data []byte) ([]CDPTarget, error) {
	var targets []CDPTarget
	if err := json.Unmarshal(data, &targets); err != nil {
		return nil, err
	}
	result := []CDPTarget{}
	for _, target := range targets {
		if target.Injectable() {
			result = append(result, target)
		}
	}
	return result, nil
}

type CDPInjectionResult struct {
	TargetCount   int               `json:"targetCount"`
	SuccessCount  int               `json:"successCount"`
	PackageErrors map[string]string `json:"packageErrors"`
}

func (r CDPInjectionResult) FailureCount() int { return r.TargetCount - r.SuccessCount }

type CDPReloadResult struct {
	TargetCount  int `json:"targetCount"`
	SuccessCount int `json:"successCount"`
}

func (r CDPReloadResult) FailureCount() int { return r.TargetCount - r.SuccessCount }

type CDPService struct {
	Endpoint      string
	AllowedOrigin string
	httpClient    *http.Client
	dialer        *websocket.Dialer
	logger        *Logger
	mu            sync.Mutex
	nextCommandID int
	broker        *CapabilityBroker
	sessions      map[string]*capabilitySession
}

func NewCDPService(logger *Logger) *CDPService {
	return &CDPService{
		Endpoint: CodexCDPTargetsURL, AllowedOrigin: CodexCDPOrigin,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		dialer:     &websocket.Dialer{HandshakeTimeout: 5 * time.Second, Proxy: http.ProxyFromEnvironment},
		logger:     logger, nextCommandID: 1,
		broker: NewCapabilityBroker(), sessions: map[string]*capabilitySession{},
	}
}

func (s *CDPService) Inject(ctx context.Context, payload Payload, forceGeneration int) (CDPInjectionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	targets, err := s.discoverTargets(ctx)
	if err != nil {
		s.closeAllCapabilitySessionsLocked()
		return CDPInjectionResult{}, err
	}
	s.reconcileCapabilitySessionsLocked(targets)
	result := CDPInjectionResult{TargetCount: len(targets), PackageErrors: map[string]string{}}
	for _, target := range targets {
		bridgeSessionID, capabilityTokens, settingsAdapter, err := s.capabilityBridgeForTargetLocked(ctx, target, payload)
		if err != nil {
			s.logError(fmt.Sprintf("目标 %s 无法建立能力通道：%v", target.ID, err))
			continue
		}
		script := injectionScriptWithCapabilities(payload, forceGeneration, bridgeSessionID, capabilityTokens, settingsAdapter)
		value, err := s.evaluate(ctx, script, *target.WebSocketDebuggerURL)
		if err != nil {
			s.logError(fmt.Sprintf("目标 %s 注入失败：%v", target.ID, err))
			continue
		}
		result.SuccessCount++
		errorsValue, _ := value["packageErrors"].([]any)
		for _, rawError := range errorsValue {
			packageError, _ := rawError.(map[string]any)
			packageID, _ := packageError["id"].(string)
			message, _ := packageError["message"].(string)
			if packageID != "" && message != "" {
				result.PackageErrors[packageID] = message
			}
		}
	}
	return result, nil
}

func (s *CDPService) CleanupAllTargets(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.closeAllCapabilitySessionsLocked()
	targets, err := s.discoverTargets(ctx)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if _, err := s.evaluate(ctx, CleanupScript, *target.WebSocketDebuggerURL); err != nil {
			s.logError(fmt.Sprintf("目标 %s 清理失败：%v", target.ID, err))
		}
	}
	return nil
}

func (s *CDPService) ReloadAllTargets(ctx context.Context) (CDPReloadResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	targets, err := s.discoverTargets(ctx)
	if err != nil {
		return CDPReloadResult{}, err
	}
	result := CDPReloadResult{TargetCount: len(targets)}
	if len(targets) == 0 {
		return result, ErrNoCodexUITargets
	}

	// Reloading invalidates every bridge token and page-scoped binding. Close the
	// old sessions before navigation so no stale capability channel survives.
	s.closeAllCapabilitySessionsLocked()
	var firstError error
	for _, target := range targets {
		debuggerURL := *target.WebSocketDebuggerURL
		// An injected package can pin the renderer in a long-running JavaScript
		// task. Best-effort termination gives Page.reload a chance to run without
		// making it a prerequisite for Chromium versions that reject the command.
		_, _ = s.execute(ctx, "Runtime.terminateExecution", map[string]any{}, debuggerURL)
		if _, err := s.execute(
			ctx,
			"Page.reload",
			map[string]any{"ignoreCache": true},
			debuggerURL,
		); err != nil {
			s.logError(fmt.Sprintf("目标 %s 界面重启失败：%v", target.ID, err))
			if firstError == nil {
				firstError = err
			}
			continue
		}
		result.SuccessCount++
	}
	if firstError != nil {
		return result, fmt.Errorf(
			"Codex 界面重启不完整（成功 %d/%d）：%w",
			result.SuccessCount,
			result.TargetCount,
			firstError,
		)
	}
	return result, nil
}

func (s *CDPService) discoverTargets(ctx context.Context) ([]CDPTarget, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		var networkError net.Error
		if errors.As(err, &networkError) || errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrCDPEndpointUnavailable
		}
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("CDP 返回了无效响应")
	}
	var targets []CDPTarget
	if err := json.NewDecoder(response.Body).Decode(&targets); err != nil {
		return nil, errors.New("CDP 返回了无效响应")
	}
	result := []CDPTarget{}
	for _, target := range targets {
		if target.Injectable() {
			result = append(result, target)
		}
	}
	return result, nil
}

func (s *CDPService) evaluate(ctx context.Context, expression, debuggerURL string) (map[string]any, error) {
	response, err := s.execute(
		ctx,
		"Runtime.evaluate",
		map[string]any{
			"expression": expression, "returnByValue": true, "awaitPromise": true, "userGesture": false,
		},
		debuggerURL,
	)
	if err != nil {
		return nil, err
	}
	if exception, ok := response["exceptionDetails"].(map[string]any); ok {
		description := ""
		if exceptionValue, ok := exception["exception"].(map[string]any); ok {
			description, _ = exceptionValue["description"].(string)
		}
		if description == "" {
			description, _ = exception["text"].(string)
		}
		if description == "" {
			description = "注入脚本执行失败"
		}
		return nil, errors.New("CDP 拒绝执行：" + description)
	}
	remoteObject, _ := response["result"].(map[string]any)
	value, _ := remoteObject["value"].(map[string]any)
	return value, nil
}

func (s *CDPService) execute(
	ctx context.Context,
	method string,
	params map[string]any,
	debuggerURL string,
) (map[string]any, error) {
	header := http.Header{}
	header.Set("Origin", s.AllowedOrigin)
	connection, _, err := s.dialer.DialContext(ctx, debuggerURL, header)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	deadline := time.Now().Add(5 * time.Second)
	_ = connection.SetWriteDeadline(deadline)
	_ = connection.SetReadDeadline(deadline)
	commandID := s.nextCommandID
	s.nextCommandID++
	command := map[string]any{
		"id": commandID, "method": method, "params": params,
	}
	if err := connection.WriteJSON(command); err != nil {
		return nil, err
	}
	for {
		_, message, err := connection.ReadMessage()
		if err != nil {
			return nil, err
		}
		var response map[string]any
		if err := json.Unmarshal(message, &response); err != nil {
			return nil, errors.New("CDP 返回了无法解析的消息")
		}
		responseID, ok := response["id"].(float64)
		if !ok || int(responseID) != commandID {
			continue
		}
		if commandError, ok := response["error"].(map[string]any); ok {
			message, _ := commandError["message"].(string)
			if message == "" {
				message = "未知错误"
			}
			return nil, errors.New("CDP 拒绝执行：" + message)
		}
		result, _ := response["result"].(map[string]any)
		return result, nil
	}
}

func (s *CDPService) logError(message string) {
	if s.logger != nil {
		s.logger.Error(message)
	}
}
