#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

RELEASE_TAG="${RELEASE_TAG:-${1:-}}"
BUILD_NUMBER="${BUILD_NUMBER:-1}"
EXPECTED_MINOS="13.0"

if [[ ! "$RELEASE_TAG" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)(-[0-9A-Za-z][0-9A-Za-z.-]*)?$ ]]; then
  echo "RELEASE_TAG 必须是 v1.2.3 或 v1.2.3-beta.1 形式" >&2
  exit 1
fi

MARKETING_VERSION="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.${BASH_REMATCH[3]}"
RELEASE_VERSION="${RELEASE_TAG#v}"

if [[ ! "$BUILD_NUMBER" =~ ^[1-9][0-9]*$ ]]; then
  echo "BUILD_NUMBER 必须是正整数" >&2
  exit 1
fi

verify_app() {
  local app_path="$1"
  local expected_archs="$2"
  local binary="$app_path/Contents/MacOS/Codex Tweaks"
  local actual_archs
  local actual_version
  local actual_release_version
  local actual_build
  local minos_values

  if [[ ! -f "$binary" ]]; then
    echo "缺少可执行文件：$binary" >&2
    return 1
  fi

  actual_archs="$(lipo -archs "$binary" | tr ' ' '\n' | sort | xargs)"
  expected_archs="$(tr ' ' '\n' <<< "$expected_archs" | sort | xargs)"
  if [[ "$actual_archs" != "$expected_archs" ]]; then
    echo "${binary} 架构错误：期望 ${expected_archs}，实际 ${actual_archs}" >&2
    return 1
  fi

  minos_values="$(vtool -show-build "$binary" | awk '$1 == "minos" { print $2 }' | sort -u)"
  if [[ -z "$minos_values" ]] || [[ "$minos_values" != "$EXPECTED_MINOS" ]]; then
    echo "${binary} 最低系统版本错误：期望 ${EXPECTED_MINOS}，实际 ${minos_values:-未知}" >&2
    return 1
  fi

  actual_version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$app_path/Contents/Info.plist")"
  actual_release_version="$(/usr/libexec/PlistBuddy -c 'Print :CodexTweaksReleaseVersion' "$app_path/Contents/Info.plist")"
  actual_build="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$app_path/Contents/Info.plist")"
  if [[ "$actual_version" != "$MARKETING_VERSION" ]] \
    || [[ "$actual_release_version" != "$RELEASE_VERSION" ]] \
    || [[ "$actual_build" != "$BUILD_NUMBER" ]]; then
    echo "${app_path} 版本错误：期望 ${RELEASE_VERSION} / ${MARKETING_VERSION} (${BUILD_NUMBER})，实际 ${actual_release_version} / ${actual_version} (${actual_build})" >&2
    return 1
  fi

  codesign --verify --deep --strict --verbose=2 "$app_path"
  echo "已校验 ${app_path}：${actual_archs}，macOS ${EXPECTED_MINOS}+"
}

verify_app "dist/Codex Tweaks.app" "arm64 x86_64"
verify_app "dist/Codex Tweaks-arm64.app" arm64
verify_app "dist/Codex Tweaks-x86_64.app" x86_64

for dmg in \
  "dist/Codex-Tweaks-${RELEASE_TAG}.dmg" \
  "dist/Codex-Tweaks-${RELEASE_TAG}-arm64.dmg" \
  "dist/Codex-Tweaks-${RELEASE_TAG}-x86_64.dmg"; do
  if [[ ! -f "$dmg" ]]; then
    echo "缺少 DMG：$dmg" >&2
    exit 1
  fi
  hdiutil verify "$dmg"
done

(
  cd dist
  shasum -a 256 -c SHA256SUMS
)

echo "Release ${RELEASE_TAG} 的全部产物校验通过"
