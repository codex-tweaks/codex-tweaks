#!/usr/bin/env bash
set -euo pipefail

RELEASE_TAG="${RELEASE_TAG:-${1:-}}"
RELEASE_BODY_PATH="${RELEASE_BODY_PATH:-${2:-release-body.md}}"
GITHUB_SERVER_URL="${GITHUB_SERVER_URL:-https://github.com}"
GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-codex-tweaks/codex-tweaks}"
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
macos_sparkle_asset="Codex-Tweaks-${RELEASE_TAG}-sparkle.zip"
windows_x64_asset="Codex-Tweaks-v${release_version}-windows-Setup-x86_64.exe"
windows_arm64_asset="Codex-Tweaks-v${release_version}-windows-Setup-arm64.exe"

if [[ -n "$RELEASE_ASSET_ROOT" ]]; then
  required_assets=(
    "${RELEASE_ASSET_ROOT}/macos/${macos_universal_asset}"
    "${RELEASE_ASSET_ROOT}/macos/${macos_arm64_asset}"
    "${RELEASE_ASSET_ROOT}/macos/${macos_x86_64_asset}"
    "${RELEASE_ASSET_ROOT}/macos/${macos_sparkle_asset}"
    "${RELEASE_ASSET_ROOT}/macos/appcast.xml"
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
## 下载

| 系统 | 下载文件 | 适用设备 |
| --- | --- | --- |
| macOS 13+ 通用版（推荐） | [${macos_universal_asset}](${download_base}/${macos_universal_asset}) | Apple Silicon（M 系列）与 Intel Mac |
| macOS 13+ Apple Silicon 版 | [${macos_arm64_asset}](${download_base}/${macos_arm64_asset}) | Apple Silicon（M 系列），下载文件更小 |
| macOS 13+ Intel 版 | [${macos_x86_64_asset}](${download_base}/${macos_x86_64_asset}) | Intel Mac，下载文件更小 |
| Windows x64 | [${windows_x64_asset}](${download_base}/${windows_x64_asset}) | 64 位 Intel 或 AMD Windows 电脑 |
| Windows ARM64 | [${windows_arm64_asset}](${download_base}/${windows_arm64_asset}) | Snapdragon 等 ARM64 Windows 电脑 |

### 安装说明

- 普通用户只需下载上表中与自己系统对应的文件。
- macOS：打开 DMG 后，将 Codex Tweaks 拖入 Applications（应用程序）文件夹。
- Windows：运行对应架构的 EXE 安装程序。
- Release 中的 Sparkle ZIP、<code>appcast.xml</code>、<code>.nupkg</code> 和 <code>releases.*.json</code> 文件供应用内自动更新使用，请勿手动下载。

---

## Downloads

| System | Download | Compatible devices |
| --- | --- | --- |
| macOS 13+ Universal (Recommended) | [${macos_universal_asset}](${download_base}/${macos_universal_asset}) | Apple Silicon (M-series) and Intel Macs |
| macOS 13+ Apple Silicon | [${macos_arm64_asset}](${download_base}/${macos_arm64_asset}) | Apple Silicon (M-series) Macs; smaller download |
| macOS 13+ Intel | [${macos_x86_64_asset}](${download_base}/${macos_x86_64_asset}) | Intel Macs; smaller download |
| Windows x64 | [${windows_x64_asset}](${download_base}/${windows_x64_asset}) | 64-bit Intel and AMD Windows PCs |
| Windows ARM64 | [${windows_arm64_asset}](${download_base}/${windows_arm64_asset}) | ARM64 Windows PCs such as Snapdragon devices |

### Installation notes

- Most users only need the file matching their system in the table above.
- macOS: Open the DMG, then drag Codex Tweaks to the Applications folder.
- Windows: Run the EXE installer matching your system architecture.
- The Sparkle ZIP, <code>appcast.xml</code>, <code>.nupkg</code>, and <code>releases.*.json</code> files are used by the in-app updaters and should not be downloaded manually.

---

EOF

echo "已生成双语 Release 下载说明：${RELEASE_BODY_PATH}"
