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

type Platform interface {
	IsCodexRunning(ctx context.Context) (bool, error)
	ActivateCodex(ctx context.Context) error
	LaunchCodex(ctx context.Context) error
	RestartCodex(ctx context.Context) error
	Architecture() string
}

// backgroundRepairPlatform is implemented by platforms that keep repairing state after the call that
// triggered them returned. Such work follows the application lifetime instead of a call context and
// reports its failures to the log.
type backgroundRepairPlatform interface {
	useBackgroundRepairContext(ctx context.Context, logger *Logger)
}
