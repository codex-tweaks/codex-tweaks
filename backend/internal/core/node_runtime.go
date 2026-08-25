package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	NodeTrustExplicit  = "explicit"
	NodeTrustAutomatic = "developerAutomatic"
)

var DependencyInstallWithScriptsArguments = []string{"ci", "--no-audit", "--no-fund"}

type NodeRuntimeEvent struct {
	PackageID string          `json:"packageID"`
	Name      string          `json:"name"`
	Payload   json.RawMessage `json:"payload"`
}

type NodeInvocationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *NodeInvocationError) Error() string { return e.Message }

type NodeInvoker interface {
	InvokeNode(context.Context, string, string, json.RawMessage) (json.RawMessage, *NodeInvocationError)
}

type nodeRuntimeResult struct {
	value json.RawMessage
	err   *NodeInvocationError
}

type nodeRuntimeProcess struct {
	packageID       string
	authorizationID string
	command         *exec.Cmd
	stdin           io.WriteCloser
	logger          *Logger
	onEvent         func(NodeRuntimeEvent)

	writeMu  sync.Mutex
	mu       sync.Mutex
	nextID   uint64
	pending  map[string]chan nodeRuntimeResult
	ready    chan struct{}
	readyErr error
	exitErr  error
	stopping bool
	done     chan struct{}
	stopOnce sync.Once
	cancel   context.CancelFunc
}

type nodeProcessMessage struct {
	Type    string               `json:"type"`
	ID      string               `json:"id,omitempty"`
	Method  string               `json:"method,omitempty"`
	Name    string               `json:"name,omitempty"`
	Payload json.RawMessage      `json:"payload,omitempty"`
	Result  json.RawMessage      `json:"result,omitempty"`
	OK      bool                 `json:"ok,omitempty"`
	Error   *NodeInvocationError `json:"error,omitempty"`
}

type NodeRuntimeSupervisor struct {
	store  *Store
	runner CommandRunner
	logger *Logger

	mu        sync.Mutex
	processes map[string]*nodeRuntimeProcess
	starting  map[string]nodeRuntimeStart
	failures  map[string]nodeRuntimeFailure
	onEvent   func(NodeRuntimeEvent)
}

type nodeRuntimeFailure struct {
	AuthorizationID string
	Message         string
}

type nodeRuntimeStart struct {
	AuthorizationID string
	Cancel          context.CancelFunc
}

func NewNodeRuntimeSupervisor(store *Store, runner CommandRunner, logger *Logger) *NodeRuntimeSupervisor {
	if runner == nil {
		runner = SystemCommandRunner{}
	}
	return &NodeRuntimeSupervisor{
		store: store, runner: runner, logger: logger,
		processes: map[string]*nodeRuntimeProcess{}, starting: map[string]nodeRuntimeStart{},
		failures: map[string]nodeRuntimeFailure{},
	}
}

func (s *NodeRuntimeSupervisor) SetEventHandler(handler func(NodeRuntimeEvent)) {
	s.mu.Lock()
	s.onEvent = handler
	s.mu.Unlock()
}

func (s *NodeRuntimeSupervisor) Reconcile(
	ctx context.Context,
	packages []Package,
	disabled map[string]bool,
	trustByPackageID map[string]string,
	environment *NodeEnvironment,
) {
	packagesByID := make(map[string]Package, len(packages))
	for _, pkg := range packages {
		packagesByID[pkg.ID] = pkg
	}
	s.mu.Lock()
	failedPackageIDs := map[string]bool{}
	for packageID, failure := range s.failures {
		if pkg, exists := packagesByID[packageID]; exists && failure.AuthorizationID == NodeAuthorizationID(pkg) {
			failedPackageIDs[packageID] = true
		}
	}
	s.mu.Unlock()
	desired := desiredNodeRuntimePackages(packages, disabled, trustByPackageID, failedPackageIDs)

	s.mu.Lock()
	for packageID, process := range s.processes {
		pkg, wanted := desired[packageID]
		if !wanted || process.authorizationID != NodeAuthorizationID(pkg) || environment == nil {
			delete(s.processes, packageID)
			go process.stop()
		}
	}
	for packageID, start := range s.starting {
		pkg, wanted := desired[packageID]
		if !wanted || start.AuthorizationID != NodeAuthorizationID(pkg) || environment == nil {
			delete(s.starting, packageID)
			start.Cancel()
		}
	}
	for packageID, failure := range s.failures {
		pkg, installed := packagesByID[packageID]
		trustedCurrent := installed && !disabled[packageID] && trustByPackageID[packageID] != "" && NodeAuthorizationID(pkg) != ""
		if !trustedCurrent || failure.AuthorizationID != NodeAuthorizationID(pkg) || environment == nil {
			delete(s.failures, packageID)
		}
	}
	if environment == nil {
		s.mu.Unlock()
		return
	}
	for packageID, pkg := range desired {
		authorizationID := NodeAuthorizationID(pkg)
		if s.processes[packageID] != nil || s.starting[packageID].AuthorizationID == authorizationID {
			continue
		}
		if failure, failed := s.failures[packageID]; failed && failure.AuthorizationID == authorizationID {
			continue
		}
		delete(s.failures, packageID)
		startContext, cancel := context.WithCancel(ctx)
		s.starting[packageID] = nodeRuntimeStart{AuthorizationID: authorizationID, Cancel: cancel}
		go s.start(startContext, pkg, *environment, authorizationID)
	}
	s.mu.Unlock()
}

func desiredNodeRuntimePackages(
	packages []Package,
	disabled map[string]bool,
	trustByPackageID map[string]string,
	failedPackageIDs map[string]bool,
) map[string]Package {
	effectiveDisabled := cloneSet(disabled)
	for _, pkg := range packages {
		if pkg.Manifest != nil && pkg.Manifest.CodexTweaks.Permissions.Node != nil &&
			(trustByPackageID[pkg.ID] == "" || NodeAuthorizationID(pkg) == "" || failedPackageIDs[pkg.ID]) {
			effectiveDisabled[pkg.ID] = true
		}
	}
	desired := map[string]Package{}
	loadablePackages := ResolveDependencies(packages, effectiveDisabled).LoadablePackages
	for _, pkg := range loadablePackages {
		if disabled[pkg.ID] || trustByPackageID[pkg.ID] == "" || NodeAuthorizationID(pkg) == "" {
			continue
		}
		desired[pkg.ID] = pkg
	}
	return desired
}

func (s *NodeRuntimeSupervisor) start(ctx context.Context, pkg Package, environment NodeEnvironment, authorizationID string) {
	if err := s.runAuthorizedInstallScripts(ctx, pkg, environment, authorizationID); err != nil {
		s.recordStartFailure(pkg.ID, authorizationID, "Node 依赖安装失败："+err.Error())
		return
	}
	dataDirectory := filepath.Join(s.store.StateDirectory, "NodeData", FingerprintString(pkg.ID)[:16])
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		s.recordStartFailure(pkg.ID, authorizationID, "无法创建 Node 数据目录："+err.Error())
		return
	}
	process, err := startNodeRuntimeProcess(ctx, pkg, environment, dataDirectory, authorizationID, s.logger, s.forwardEvent)
	if err != nil {
		s.recordStartFailure(pkg.ID, authorizationID, err.Error())
		return
	}

	s.mu.Lock()
	if s.starting[pkg.ID].AuthorizationID != authorizationID {
		s.mu.Unlock()
		process.stop()
		return
	}
	delete(s.starting, pkg.ID)
	s.processes[pkg.ID] = process
	delete(s.failures, pkg.ID)
	s.mu.Unlock()
	if s.logger != nil {
		s.logger.Info("Node 功能包已启动：" + pkg.DisplayName())
	}
	go func() {
		<-process.done
		s.mu.Lock()
		if s.processes[pkg.ID] == process {
			delete(s.processes, pkg.ID)
			if terminalError := process.terminalError(); terminalError != nil {
				s.failures[pkg.ID] = nodeRuntimeFailure{AuthorizationID: authorizationID, Message: terminalError.Error()}
			}
		}
		s.mu.Unlock()
	}()
}

func (s *NodeRuntimeSupervisor) runAuthorizedInstallScripts(
	ctx context.Context,
	pkg Package,
	environment NodeEnvironment,
	authorizationID string,
) error {
	lockPath := filepath.Join(pkg.Directory, "package-lock.json")
	if !isRegularNonSymlink(lockPath) {
		return nil
	}
	markerDirectory := filepath.Join(s.store.StateDirectory, "NodeInstallAuthorizations")
	markerPath := filepath.Join(markerDirectory, authorizationID+".json")
	if isRegularNonSymlink(markerPath) {
		return nil
	}
	if err := os.MkdirAll(markerDirectory, 0o700); err != nil {
		return err
	}
	environmentValues := processEnvironment(filepath.Dir(environment.NodePath), environmentMap())
	result, err := s.runner.Run(ctx, environment.NPMPath, DependencyInstallWithScriptsArguments, pkg.Directory, environmentValues)
	if err != nil {
		return err
	}
	if err := requireCommandSuccess(result, "npm ci"); err != nil {
		return err
	}
	return writeJSONAtomic(markerPath, map[string]any{
		"packageID": pkg.ID, "authorizationID": authorizationID,
		"completedAt": NewCodableTime(time.Now()),
	})
}

func (s *NodeRuntimeSupervisor) recordStartFailure(packageID, authorizationID, message string) {
	s.mu.Lock()
	start := s.starting[packageID]
	recorded := false
	if s.starting[packageID].AuthorizationID == authorizationID {
		delete(s.starting, packageID)
		s.failures[packageID] = nodeRuntimeFailure{AuthorizationID: authorizationID, Message: message}
		recorded = true
	}
	s.mu.Unlock()
	if start.AuthorizationID == authorizationID && start.Cancel != nil {
		start.Cancel()
	}
	if recorded && s.logger != nil {
		s.logger.Error("Node 功能包 " + packageID + " 启动失败：" + message)
	}
}

func (s *NodeRuntimeSupervisor) forwardEvent(event NodeRuntimeEvent) {
	s.mu.Lock()
	handler := s.onEvent
	s.mu.Unlock()
	if handler != nil {
		handler(event)
	}
}

func (s *NodeRuntimeSupervisor) InvokeNode(
	ctx context.Context,
	packageID, method string,
	parameters json.RawMessage,
) (json.RawMessage, *NodeInvocationError) {
	s.mu.Lock()
	process := s.processes[packageID]
	s.mu.Unlock()
	if process == nil {
		return nil, &NodeInvocationError{Code: "node_unavailable", Message: "Node runtime is not running"}
	}
	return process.invoke(ctx, method, parameters)
}

func (s *NodeRuntimeSupervisor) RuntimeErrors() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string, len(s.failures))
	for packageID, failure := range s.failures {
		result[packageID] = failure.Message
	}
	return result
}

func (s *NodeRuntimeSupervisor) RunningPackageIDs() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := map[string]bool{}
	for packageID := range s.processes {
		result[packageID] = true
	}
	return result
}

func (s *NodeRuntimeSupervisor) StopPackage(packageID string) {
	s.mu.Lock()
	process := s.processes[packageID]
	start := s.starting[packageID]
	delete(s.processes, packageID)
	delete(s.starting, packageID)
	delete(s.failures, packageID)
	s.mu.Unlock()
	if start.Cancel != nil {
		start.Cancel()
	}
	if process != nil {
		process.stop()
	}
}

func (s *NodeRuntimeSupervisor) StopAll() {
	s.mu.Lock()
	processes := make([]*nodeRuntimeProcess, 0, len(s.processes))
	for _, process := range s.processes {
		processes = append(processes, process)
	}
	s.processes = map[string]*nodeRuntimeProcess{}
	starts := make([]nodeRuntimeStart, 0, len(s.starting))
	for _, start := range s.starting {
		starts = append(starts, start)
	}
	s.starting = map[string]nodeRuntimeStart{}
	s.failures = map[string]nodeRuntimeFailure{}
	s.mu.Unlock()
	for _, start := range starts {
		start.Cancel()
	}
	for _, process := range processes {
		process.stop()
	}
}

func startNodeRuntimeProcess(
	ctx context.Context,
	pkg Package,
	environment NodeEnvironment,
	dataDirectory, authorizationID string,
	logger *Logger,
	onEvent func(NodeRuntimeEvent),
) (*nodeRuntimeProcess, error) {
	if pkg.ActiveBuild == nil {
		return nil, errors.New("Node 编译产物不存在。")
	}
	processContext, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(processContext, environment.NodePath, "-e", nodeRuntimeRunnerSource,
		pkg.ActiveBuild.NodeJavaScriptPath(), pkg.Directory, dataDirectory)
	command.Dir = pkg.Directory
	environmentValues := environmentMap()
	nodeModules := filepath.Join(pkg.Directory, "node_modules")
	existingNodePath := environmentValue(environmentValues, "NODE_PATH")
	if existingNodePath != "" {
		nodeModules += string(os.PathListSeparator) + existingNodePath
	}
	setEnvironmentValue(environmentValues, "NODE_PATH", nodeModules)
	command.Env = processEnvironment(filepath.Dir(environment.NodePath), environmentValues)
	stdin, err := command.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	process := &nodeRuntimeProcess{
		packageID: pkg.ID, authorizationID: authorizationID, command: command, stdin: stdin,
		logger: logger, onEvent: onEvent, pending: map[string]chan nodeRuntimeResult{},
		ready: make(chan struct{}), done: make(chan struct{}), cancel: cancel,
	}
	if err := command.Start(); err != nil {
		cancel()
		return nil, err
	}
	go process.readStdout(stdout)
	go process.readStderr(stderr)
	go func() {
		err := command.Wait()
		cancel()
		process.finish(err)
	}()
	select {
	case <-process.ready:
		if err := process.readyError(); err != nil {
			process.stop()
			return nil, err
		}
		return process, nil
	case <-time.After(15 * time.Second):
		process.stop()
		return nil, errors.New("Node runtime 启动超时。")
	case <-ctx.Done():
		process.stop()
		return nil, ctx.Err()
	}
}

func (p *nodeRuntimeProcess) readStdout(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		message := nodeProcessMessage{}
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			if p.logger != nil {
				p.logger.Error("Node 功能包 " + p.packageID + " 输出了无效协议消息")
			}
			continue
		}
		switch message.Type {
		case "ready":
			p.markReady(nil)
		case "response":
			p.mu.Lock()
			waiter := p.pending[message.ID]
			delete(p.pending, message.ID)
			p.mu.Unlock()
			if waiter != nil {
				waiter <- nodeRuntimeResult{value: message.Result, err: message.Error}
			}
		case "event":
			if message.Name != "" && p.onEvent != nil {
				p.onEvent(NodeRuntimeEvent{PackageID: p.packageID, Name: message.Name, Payload: message.Payload})
			}
		case "fatal":
			if message.Error == nil {
				message.Error = &NodeInvocationError{Code: "node_start_failed", Message: "Node runtime failed to start"}
			}
			p.markReady(message.Error)
		}
	}
}

func (p *nodeRuntimeProcess) readStderr(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 16*1024), 1024*1024)
	for scanner.Scan() {
		if p.logger != nil {
			p.logger.Info("Node[" + p.packageID + "] " + scanner.Text())
		}
	}
}

func (p *nodeRuntimeProcess) markReady(err error) {
	p.mu.Lock()
	select {
	case <-p.ready:
		p.mu.Unlock()
		return
	default:
	}
	p.readyErr = err
	close(p.ready)
	p.mu.Unlock()
}

func (p *nodeRuntimeProcess) readyError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.readyErr
}

func (p *nodeRuntimeProcess) terminalError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.readyErr != nil {
		return p.readyErr
	}
	if p.stopping {
		return nil
	}
	if p.exitErr != nil {
		return p.exitErr
	}
	return errors.New("Node runtime exited unexpectedly")
}

func (p *nodeRuntimeProcess) finish(processError error) {
	p.mu.Lock()
	p.exitErr = processError
	select {
	case <-p.ready:
	default:
		if processError == nil {
			processError = errors.New("Node runtime exited before activation")
		}
		p.readyErr = processError
		close(p.ready)
	}
	pending := p.pending
	p.pending = map[string]chan nodeRuntimeResult{}
	select {
	case <-p.done:
		p.mu.Unlock()
		return
	default:
		close(p.done)
	}
	p.mu.Unlock()
	for _, waiter := range pending {
		waiter <- nodeRuntimeResult{err: &NodeInvocationError{Code: "node_stopped", Message: "Node runtime stopped"}}
	}
}

func (p *nodeRuntimeProcess) invoke(ctx context.Context, method string, parameters json.RawMessage) (json.RawMessage, *NodeInvocationError) {
	method = strings.TrimSpace(method)
	if method == "" || len(method) > 200 {
		return nil, &NodeInvocationError{Code: "invalid_request", Message: "Node method is invalid"}
	}
	if len(parameters) == 0 {
		parameters = json.RawMessage("null")
	}
	p.mu.Lock()
	p.nextID++
	requestID := fmt.Sprintf("%d", p.nextID)
	waiter := make(chan nodeRuntimeResult, 1)
	p.pending[requestID] = waiter
	p.mu.Unlock()
	message, _ := json.Marshal(nodeProcessMessage{Type: "request", ID: requestID, Method: method, Payload: parameters})
	if err := p.writeLine(message); err != nil {
		p.mu.Lock()
		delete(p.pending, requestID)
		p.mu.Unlock()
		return nil, &NodeInvocationError{Code: "node_unavailable", Message: err.Error()}
	}
	select {
	case result := <-waiter:
		return result.value, result.err
	case <-ctx.Done():
		p.mu.Lock()
		delete(p.pending, requestID)
		p.mu.Unlock()
		return nil, &NodeInvocationError{Code: "timeout", Message: ctx.Err().Error()}
	case <-p.done:
		return nil, &NodeInvocationError{Code: "node_stopped", Message: "Node runtime stopped"}
	}
}

func (p *nodeRuntimeProcess) writeLine(message []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err := p.stdin.Write(append(message, '\n'))
	return err
}

func (p *nodeRuntimeProcess) stop() {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		p.stopping = true
		p.mu.Unlock()
		_ = p.writeLine([]byte(`{"type":"shutdown"}`))
		select {
		case <-p.done:
		case <-time.After(2 * time.Second):
			if p.command.Process != nil {
				_ = p.command.Process.Kill()
			}
		}
		if p.cancel != nil {
			p.cancel()
		}
	})
}

const nodeRuntimeRunnerSource = `
const readline = require("node:readline");
const [bundlePath, packageDirectory, dataDirectory] = process.argv.slice(1);
const handlers = new Map();
const abortController = new AbortController();
let cleanup;
let shuttingDown = false;

const send = (value) => process.stdout.write(JSON.stringify(value) + "\n");
const safeError = (error, code = "node_error") => ({
  code: typeof error?.code === "string" ? error.code : code,
  message: error instanceof Error ? error.message : String(error),
});
const writeLog = (...values) => process.stderr.write(values.map((value) =>
  typeof value === "string" ? value : JSON.stringify(value)
).join(" ") + "\n");
console.log = writeLog;
console.info = writeLog;
console.warn = writeLog;
console.error = writeLog;

const rpc = Object.freeze({
  handle(method, handler) {
    if (typeof method !== "string" || !method.trim() || typeof handler !== "function") {
      throw new TypeError("rpc.handle(method, handler) requires a method and function");
    }
    const normalizedMethod = method.trim();
    if (handlers.has(normalizedMethod)) throw new Error("Node RPC handler already registered: " + normalizedMethod);
    handlers.set(normalizedMethod, handler);
    return () => handlers.delete(normalizedMethod);
  },
  emit(name, payload = null) {
    if (typeof name !== "string" || !name.trim()) throw new TypeError("rpc.emit name is required");
    send({ type: "event", name: name.trim(), payload });
  },
});

async function shutdown() {
  if (shuttingDown) return;
  shuttingDown = true;
  abortController.abort();
  try {
    if (typeof cleanup === "function") await cleanup();
    else if (cleanup && typeof cleanup.dispose === "function") await cleanup.dispose();
  } catch (error) {
    writeLog("cleanup failed:", error?.stack || error);
  }
  process.exit(0);
}

const input = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
input.on("line", async (line) => {
  let message;
  try { message = JSON.parse(line); } catch { return; }
  if (message.type === "shutdown") return shutdown();
  if (message.type !== "request") return;
  const handler = handlers.get(message.method);
  if (!handler) {
    send({ type: "response", id: message.id, ok: false, error: { code: "method_not_found", message: "Unknown Node RPC method: " + message.method } });
    return;
  }
  try {
    const result = await handler(message.payload);
    send({ type: "response", id: message.id, ok: true, result: result === undefined ? null : result });
  } catch (error) {
    send({ type: "response", id: message.id, ok: false, error: safeError(error) });
  }
});
input.on("close", shutdown);
process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);

(async () => {
  try {
    const moduleValue = require(bundlePath);
    const activate = moduleValue.activate || moduleValue.default?.activate || moduleValue.default;
    if (typeof activate !== "function") throw new Error("Node entry must export activate(context)");
    cleanup = await activate(Object.freeze({ rpc, packageDirectory, dataDirectory, signal: abortController.signal }));
    send({ type: "ready" });
  } catch (error) {
    send({ type: "fatal", error: safeError(error, "node_start_failed") });
    setTimeout(() => process.exit(1), 10);
  }
})();
`
