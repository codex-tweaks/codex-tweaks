#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

RELEASE_TAG="${RELEASE_TAG:-${1:-}}"
BUILD_NUMBER="${BUILD_NUMBER:-1}"
DIST_DIR="${DIST_DIR:-dist}"

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

PRODUCT_NAME="Codex Tweaks"
CODE_SIGN_IDENTITY="${MACOS_CODE_SIGN_IDENTITY:--}"

SIGNING_SETTINGS=("CODE_SIGN_IDENTITY=${CODE_SIGN_IDENTITY}")
if [[ "$CODE_SIGN_IDENTITY" != "-" ]]; then
  if [[ -z "${MACOS_SIGNING_KEYCHAIN:-}" ]]; then
    echo "使用稳定 macOS 签名时必须提供 MACOS_SIGNING_KEYCHAIN。" >&2
    exit 1
  fi
  SIGNING_SETTINGS+=(
    "MACOS_SIGNING_KEYCHAIN=${MACOS_SIGNING_KEYCHAIN}"
    "OTHER_CODE_SIGN_FLAGS=--keychain ${MACOS_SIGNING_KEYCHAIN} --timestamp=none"
  )
fi
if [[ -n "${SPARKLE_PUBLIC_ED_KEY:-}" ]]; then
  SIGNING_SETTINGS+=("SPARKLE_PUBLIC_ED_KEY=${SPARKLE_PUBLIC_ED_KEY}")
fi

mkdir -p "$DIST_DIR"
rm -rf \
  "${DIST_DIR}/${PRODUCT_NAME}.app" \
  "${DIST_DIR}/${PRODUCT_NAME}-arm64.app" \
  "${DIST_DIR}/${PRODUCT_NAME}-x86_64.app"
rm -f \
  "${DIST_DIR}/Codex-Tweaks-${RELEASE_TAG}.dmg" \
  "${DIST_DIR}/Codex-Tweaks-${RELEASE_TAG}-arm64.dmg" \
  "${DIST_DIR}/Codex-Tweaks-${RELEASE_TAG}-x86_64.dmg" \
  "${DIST_DIR}/SHA256SUMS"

build_app() {
  local label="$1"
  local archs="$2"
  local output_name="$3"
  local derived_data="build/DerivedData-Release-${label}"

  rm -rf "$derived_data"
  xcodebuild \
    -quiet \
    -project CodexTweaks.xcodeproj \
    -scheme CodexTweaks \
    -configuration Release \
    -destination "generic/platform=macOS" \
    -derivedDataPath "$derived_data" \
    -packageAuthorizationProvider netrc \
    ARCHS="$archs" \
    ONLY_ACTIVE_ARCH=NO \
    MARKETING_VERSION="$MARKETING_VERSION" \
    CODEX_TWEAKS_RELEASE_VERSION="$RELEASE_VERSION" \
    CURRENT_PROJECT_VERSION="$BUILD_NUMBER" \
    "${SIGNING_SETTINGS[@]}" \
    build

  ditto \
    "$derived_data/Build/Products/Release/${PRODUCT_NAME}.app" \
    "${DIST_DIR}/${output_name}.app"
}

create_dmg() (
  local app_name="$1"
  local dmg_name="$2"
  local settings_file

  settings_file="$(mktemp "${TMPDIR:-/tmp}/codex-tweaks-dmgbuild.XXXXXX.json")"
  trap 'rm -f "$settings_file"' EXIT
  sed \
    "s|dist/Codex Tweaks-arm64.app|${DIST_DIR}/${app_name}.app|" \
    scripts/dmgbuild.json \
    > "$settings_file"

  dmgbuild \
    -s "$settings_file" \
    "$PRODUCT_NAME" \
    "${DIST_DIR}/${dmg_name}"

  if [[ "$CODE_SIGN_IDENTITY" != "-" ]]; then
    codesign \
      --force \
      --sign "$CODE_SIGN_IDENTITY" \
      --keychain "$MACOS_SIGNING_KEYCHAIN" \
      --timestamp=none \
      "${DIST_DIR}/${dmg_name}"
  fi

  echo "已创建 ${DIST_DIR}/${dmg_name}"
)

build_app universal "arm64 x86_64" "$PRODUCT_NAME"
build_app arm64 arm64 "${PRODUCT_NAME}-arm64"
build_app x86_64 x86_64 "${PRODUCT_NAME}-x86_64"

create_dmg "$PRODUCT_NAME" "Codex-Tweaks-${RELEASE_TAG}.dmg"
create_dmg "${PRODUCT_NAME}-arm64" "Codex-Tweaks-${RELEASE_TAG}-arm64.dmg"
create_dmg "${PRODUCT_NAME}-x86_64" "Codex-Tweaks-${RELEASE_TAG}-x86_64.dmg"

echo "Release ${RELEASE_TAG} 构建完成（版本 ${MARKETING_VERSION}，构建号 ${BUILD_NUMBER}）"
