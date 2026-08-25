package rpc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/cr-zhichen/codex-tweaks/backend/internal/core"
)

type request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type response struct {
	ID     int64     `json:"id"`
	Result any       `json:"result,omitempty"`
	Error  *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type event struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

type Server struct {
	reader       io.Reader
	writer       io.Writer
	writeMu      sync.Mutex
	controller   *core.Controller
	dependencies core.ControllerDependencies
}

func NewServer(reader io.Reader, writer io.Writer) *Server {
	return &Server{reader: reader, writer: writer}
}

func NewServerWithDependencies(
	reader io.Reader,
	writer io.Writer,
	dependencies core.ControllerDependencies,
) *Server {
	return &Server{
		reader: reader, writer: writer, dependencies: dependencies,
	}
}

func (s *Server) Serve() error {
	scanner := bufio.NewScanner(s.reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var incoming request
		if err := json.Unmarshal(scanner.Bytes(), &incoming); err != nil {
			s.write(response{Error: &rpcError{Code: "invalid_request", Message: err.Error()}})
			continue
		}
		if incoming.ID == 0 || incoming.Method == "" {
			s.write(response{ID: incoming.ID, Error: &rpcError{Code: "invalid_request", Message: "id 和 method 不能为空"}})
			continue
		}
		result, err := s.dispatch(incoming)
		if err != nil {
			s.write(response{ID: incoming.ID, Error: &rpcError{Code: "request_failed", Message: err.Error()}})
		} else {
			s.write(response{ID: incoming.ID, Result: result})
		}
		if incoming.Method == "shutdown" {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if s.controller != nil {
		s.controller.Shutdown()
	}
	return nil
}

func (s *Server) dispatch(incoming request) (any, error) {
	if incoming.Method == "ping" {
		return map[string]any{"protocolVersion": core.ProtocolVersion, "backend": "go"}, nil
	}
	if incoming.Method == "initialize" {
		if s.controller != nil {
			return nil, errors.New("后端已经初始化")
		}
		var params core.InitializeParams
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		controller, err := core.NewController(params, func(snapshot core.AppSnapshot) {
			s.write(event{Event: "state", Data: snapshot})
		}, s.dependencies)
		if err != nil {
			return nil, err
		}
		s.controller = controller
		return controller.Snapshot(), nil
	}
	if s.controller == nil {
		return nil, errors.New("请先调用 initialize")
	}
	c := s.controller
	switch incoming.Method {
	case "getState":
		return c.Snapshot(), nil
	case "setEnabled":
		var params struct {
			Enabled bool `json:"enabled"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.SetEnabled(params.Enabled))
	case "setDeveloperMode":
		var params struct {
			Enabled bool `json:"enabled"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.SetDeveloperMode(params.Enabled))
	case "setDeveloperAllowUnknownNode":
		var params struct {
			Enabled bool `json:"enabled"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.SetDeveloperAllowUnknownNode(params.Enabled))
	case "authorizeNodePackage":
		var params struct {
			PackageID       string `json:"packageID"`
			AuthorizationID string `json:"authorizationID"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.AuthorizeNodePackage(params.PackageID, params.AuthorizationID))
	case "setPackageEnabled":
		var params struct {
			PackageID string `json:"packageID"`
			Enabled   bool   `json:"enabled"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.SetPackageEnabled(params.PackageID, params.Enabled))
	case "setPackagePriority":
		var params struct {
			PackageID string `json:"packageID"`
			Priority  *int   `json:"priority"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.SetPackagePriority(params.PackageID, params.Priority))
	case "enableDependencies":
		var params packageIDParams
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.EnableDependencies(params.PackageID))
	case "reloadPackages":
		return accepted(c.ReloadPackages())
	case "checkNodeEnvironment":
		c.CheckNodeEnvironment()
		return accepted(nil)
	case "checkGitEnvironment":
		c.CheckGitEnvironment()
		return accepted(nil)
	case "buildPackage":
		var params packageIDParams
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.BuildPackage(params.PackageID))
	case "exportPackage":
		var params struct {
			PackageID       string `json:"packageID"`
			DestinationPath string `json:"destinationPath"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.ExportPackage(params.PackageID, params.DestinationPath))
	case "installRemotePackage":
		var params struct {
			RepositoryURL string                  `json:"repositoryURL"`
			SelectorType  core.RemoteSelectorType `json:"selectorType"`
			SelectorValue string                  `json:"selectorValue"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		c.InstallRemotePackage(params.RepositoryURL, params.SelectorType, params.SelectorValue)
		return accepted(nil)
	case "installLocalPackage":
		var params struct {
			SourcePath string `json:"sourcePath"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		c.InstallLocalPackage(params.SourcePath)
		return accepted(nil)
	case "reportLocalPackageSelectionError":
		var params struct {
			Message string `json:"message"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		c.ReportLocalPackageSelectionError(params.Message)
		return accepted(nil)
	case "clearRemoteOperationFeedback":
		c.ClearRemoteOperationFeedback()
		return accepted(nil)
	case "clearLocalOperationFeedback":
		c.ClearLocalOperationFeedback()
		return accepted(nil)
	case "installMissingDependencies":
		var params packageIDParams
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.InstallMissingDependencies(params.PackageID))
	case "checkManagedPackageUpdates":
		var params struct {
			Automatic bool `json:"automatic"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		c.CheckManagedPackageUpdates(params.Automatic)
		return accepted(nil)
	case "updateManagedPackage":
		var params packageIDParams
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.UpdateManagedPackage(params.PackageID))
	case "openCodex":
		return accepted(c.OpenCodex())
	case "restartCodex":
		c.RestartCodex()
		return accepted(nil)
	case "restartCodexUI":
		return accepted(c.RestartCodexUI())
	case "reinject":
		c.Reinject()
		return accepted(nil)
	case "refreshLog":
		c.RefreshLog()
		return accepted(nil)
	case "clearLog":
		return accepted(c.ClearLog())
	case "readAuthoringPrompt":
		return c.ReadAuthoringPrompt()
	case "checkAppUpdate":
		var params struct {
			Prompt bool `json:"prompt"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		c.CheckAppUpdate(params.Prompt)
		return accepted(nil)
	case "setUpdateChannel":
		var params struct {
			Channel core.UpdateChannel `json:"channel"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.SetUpdateChannel(params.Channel))
	case "setUpdateAutoCheck":
		var params struct {
			Enabled bool `json:"enabled"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.SetUpdateAutoCheck(params.Enabled))
	case "dismissUpdate":
		c.DismissUpdate()
		return accepted(nil)
	case "skipUpdate":
		var params struct {
			TagName string `json:"tagName"`
		}
		if err := decodeParams(incoming.Params, &params); err != nil {
			return nil, err
		}
		return accepted(c.SkipUpdate(params.TagName))
	case "unskipAndPromptUpdate":
		return accepted(c.UnskipAndPromptUpdate())
	case "shutdown":
		c.Shutdown()
		return map[string]bool{"shutdown": true}, nil
	default:
		return nil, fmt.Errorf("未知方法：%s", incoming.Method)
	}
}

type packageIDParams struct {
	PackageID string `json:"packageID"`
}

func decodeParams(data json.RawMessage, destination any) error {
	if len(data) == 0 || string(data) == "null" {
		data = []byte("{}")
	}
	return json.Unmarshal(data, destination)
}

func accepted(err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]bool{"accepted": true}, nil
}

func (s *Server) write(value any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = s.writer.Write(append(data, '\n'))
}
