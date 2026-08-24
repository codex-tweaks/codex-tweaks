#!/usr/bin/env bash
set -euo pipefail

RELEASE_TAG="${RELEASE_TAG:-${1:-}}"
RELEASE_BODY_PATH="${RELEASE_BODY_PATH:-${2:-release-body.md}}"
GITHUB_SERVER_URL="${GITHUB_SERVER_URL:-https://github.com}"
GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-cr-zhichen/codex-tweaks}"
RELEASE_ASSET_ROOT="${RELEASE_ASSET_ROOT:-}"

if [[ ! "$RELEASE_TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$ ]]; then
  echo "RELEASE_TAG 必须是 v1.2.3 或 v1.2.3-beta.1 形式" >&2
  exit 1
fi

release_version="${RELEASE_TAG#v}"
download_base="${GITHUB_SERVER_URL%/}/${GITHUB_REPOSITORY}/releases/download/${RELEASE_TAG}"
macos_universal_asset="Codex-Tweaks-${RELEASE_TAG}.dmg"
macos_arm64_asset="Codex-Tweaks-${RELEASE_TAG}-arm64.dmg"
macos_x86_64_asset="Codex-Tweaks-${RELEASE_TAG}-x86_64.dmg"
windows_x64_asset="Codex-Tweaks-v${release_version}-windows-Setup-x86_64.exe"
windows_arm64_asset="Codex-Tweaks-v${release_version}-windows-Setup-arm64.exe"

if [[ -n "$RELEASE_ASSET_ROOT" ]]; then
  required_assets=(
    "${RELEASE_ASSET_ROOT}/macos/${macos_universal_asset}"
    "${RELEASE_ASSET_ROOT}/macos/${macos_arm64_asset}"
    "${RELEASE_ASSET_ROOT}/macos/${macos_x86_64_asset}"
    "${RELEASE_ASSET_ROOT}/windows/${windows_x64_asset}"
    "${RELEASE_ASSET_ROOT}/windows/${windows_arm64_asset}"
  )
  for asset in "${required_assets[@]}"; do
    if [[ ! -f "$asset" ]]; then
      echo "发布说明引用的文件不存在：$asset" >&2
      exit 1
    fi
  done
fi

mkdir -p "$(dirname -- "$RELEASE_BODY_PATH")"
cat > "$RELEASE_BODY_PATH" <<EOF
## 下载 / Downloads

| 系统 / System | 下载 / Download | 适用于 / For |
| --- | --- | --- |
| macOS 13+ universal（推荐 / Recommended） | [${macos_universal_asset}](${download_base}/${macos_universal_asset}) | Apple Silicon（M 系列）与 Intel Mac / Apple Silicon and Intel Macs |
| macOS 13+ Apple Silicon | [${macos_arm64_asset}](${download_base}/${macos_arm64_asset}) | Apple Silicon（M 系列），文件更小 / Apple Silicon Macs, smaller download |
| macOS 13+ Intel | [${macos_x86_64_asset}](${download_base}/${macos_x86_64_asset}) | Intel Mac，文件更小 / Intel Macs, smaller download |
| Windows x64 | [${windows_x64_asset}](${download_base}/${windows_x64_asset}) | Intel、AMD 64 位 Windows 电脑 / 64-bit Intel and AMD Windows PCs |
| Windows ARM64 | [${windows_arm64_asset}](${download_base}/${windows_arm64_asset}) | Snapdragon 等 ARM64 Windows 电脑 / ARM64 Windows PCs such as Snapdragon devices |

### 安装提示 / Installation notes

- 中文：普通用户只需下载上表中与自己系统对应的文件。macOS 打开 DMG 后拖入 Applications（应用程序）；Windows 运行对应的 EXE 安装器。
- English: Most users only need the matching file in the table. On macOS, open the DMG and drag the app to Applications. On Windows, run the matching EXE installer.
- 中文：Release 中的 <code>.nupkg</code> 和 <code>releases.*.json</code> 供 Windows 应用内自动更新使用，请勿手动下载。
- English: The <code>.nupkg</code> and <code>releases.*.json</code> assets are used by the Windows in-app updater and should not be downloaded manually.

EOF

echo "已生成双语 Release 下载说明：${RELEASE_BODY_PATH}"
