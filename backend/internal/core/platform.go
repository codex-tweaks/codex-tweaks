package core

import "context"

const CodexBundleIdentifier = "com.openai.codex"

const (
	CodexCDPEndpoint   = "127.0.0.1:9335"
	CodexCDPOrigin     = "http://" + CodexCDPEndpoint
	CodexCDPTargetsURL = CodexCDPOrigin + "/json/list"
)

var CodexDebuggingArguments = []string{
	"--remote-debugging-address=127.0.0.1",
	"--remote-debugging-port=9335",
	"--remote-allow-origins=" + CodexCDPOrigin,
}

type CodexLaunchOptions struct {
	DisableGPUAcceleration bool
}

func codexLaunchArguments(options CodexLaunchOptions, operatingSystem string) []string {
	arguments := append([]string(nil), CodexDebuggingArguments...)
	if !options.DisableGPUAcceleration {
		return arguments
	}
	if operatingSystem == "darwin" {
		arguments = append(arguments, "--use-gl=angle", "--use-angle=swiftshader")
	} else {
		arguments = append(arguments, "--disable-gpu")
	}
	return arguments
}

type Platform interface {
	IsCodexRunning(ctx context.Context) (bool, error)
	ActivateCodex(ctx context.Context) error
	LaunchCodex(ctx context.Context, options CodexLaunchOptions) error
	RestartCodex(ctx context.Context, options CodexLaunchOptions) error
	Architecture() string
}
