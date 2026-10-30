package core

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Read the installed resource index, including lazy modules that have never
// appeared in Debugger.scriptParsed. No application code is loaded or patched.
func installedCodexAssetURLs(ctx context.Context) ([]string, error) {
	runner := SystemCommandRunner{}
	var result CommandResult
	var err error
	switch runtime.GOOS {
	case "darwin":
		result, err = runner.Run(ctx, "/usr/bin/osascript", []string{
			"-e", `POSIX path of (path to application id "` + CodexBundleIdentifier + `")`,
		}, "", environmentSlice(environmentMap()))
	case "windows":
		result, err = runner.Run(ctx, "powershell.exe", []string{
			"-NoProfile", "-NonInteractive", "-Command",
			`Get-Process -Name ChatGPT | Where-Object { $_.Path } | Select-Object -First 1 -ExpandProperty Path`,
		}, "", environmentSlice(environmentMap()))
	default:
		return nil, errors.New("当前系统不支持读取 Codex 资源，请在 macOS 或 Windows 中使用")
	}
	if err != nil {
		return nil, err
	}
	if err := requireCommandSuccess(result, "定位 Codex 应用"); err != nil {
		return nil, err
	}
	appPath := strings.TrimSpace(strings.TrimPrefix(result.Output, "\ufeff"))
	if appPath == "" {
		return nil, errors.New("没有找到 Codex 应用，请启动 Codex 后重试")
	}
	archivePath := filepath.Join(appPath, "Contents", "Resources", "app.asar")
	if runtime.GOOS == "windows" {
		archivePath = filepath.Join(filepath.Dir(appPath), "resources", "app.asar")
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return codexAssetURLs(file)
}

func codexAssetURLs(reader io.ReaderAt) ([]string, error) {
	// ASAR stores a Pickle-encoded header size followed by a Pickle JSON string.
	var prefix [16]byte
	if _, err := reader.ReadAt(prefix[:], 0); err != nil {
		return nil, err
	}
	headerSize := binary.LittleEndian.Uint32(prefix[4:8])
	jsonSize := binary.LittleEndian.Uint32(prefix[12:16])
	if headerSize < 8 || jsonSize > headerSize-8 {
		return nil, errors.New("Codex 资源目录损坏，请重新安装 Codex 后重试")
	}
	type entry struct {
		Files map[string]entry `json:"files"`
	}
	var directory entry
	if err := json.NewDecoder(io.NewSectionReader(reader, 16, int64(jsonSize))).Decode(&directory); err != nil {
		return nil, err
	}
	assets := directory.Files["webview"].Files["assets"].Files
	urls := make([]string, 0, len(assets))
	for name := range assets {
		if strings.HasSuffix(name, ".js") {
			urls = append(urls, "app://-/assets/"+name)
		}
	}
	return urls, nil
}
