#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

RELEASE_TAG="${RELEASE_TAG:-${1:-}}"
BUILD_NUMBER="${BUILD_NUMBER:-1}"

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

mkdir -p dist
rm -rf \
  "dist/${PRODUCT_NAME}.app" \
  "dist/${PRODUCT_NAME}-arm64.app" \
  "dist/${PRODUCT_NAME}-x86_64.app"
rm -f \
  "dist/Codex-Tweaks-${RELEASE_TAG}.dmg" \
  "dist/Codex-Tweaks-${RELEASE_TAG}-arm64.dmg" \
  "dist/Codex-Tweaks-${RELEASE_TAG}-x86_64.dmg" \
  dist/SHA256SUMS

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
    ARCHS="$archs" \
    ONLY_ACTIVE_ARCH=NO \
    CODE_SIGN_IDENTITY="-" \
    MARKETING_VERSION="$MARKETING_VERSION" \
    CODEX_TWEAKS_RELEASE_VERSION="$RELEASE_VERSION" \
    CURRENT_PROJECT_VERSION="$BUILD_NUMBER" \
    build

  ditto \
    "$derived_data/Build/Products/Release/${PRODUCT_NAME}.app" \
    "dist/${output_name}.app"
}

create_dmg() (
  local app_name="$1"
  local dmg_name="$2"
  local staging_dir

  staging_dir="$(mktemp -d "${TMPDIR:-/tmp}/codex-tweaks-dmg.XXXXXX")"
  trap 'rm -rf "$staging_dir"' EXIT
  ditto "dist/${app_name}.app" "$staging_dir/${PRODUCT_NAME}.app"
  ln -s /Applications "$staging_dir/Applications"

  COPYFILE_DISABLE=1 hdiutil create \
    -quiet \
    -ov \
    -fs HFS+ \
    -format UDZO \
    -volname "$PRODUCT_NAME" \
    -srcfolder "$staging_dir" \
    "dist/${dmg_name}"

  echo "已创建 dist/${dmg_name}"
)

build_app universal "arm64 x86_64" "$PRODUCT_NAME"
build_app arm64 arm64 "${PRODUCT_NAME}-arm64"
build_app x86_64 x86_64 "${PRODUCT_NAME}-x86_64"

create_dmg "$PRODUCT_NAME" "Codex-Tweaks-${RELEASE_TAG}.dmg"
create_dmg "${PRODUCT_NAME}-arm64" "Codex-Tweaks-${RELEASE_TAG}-arm64.dmg"
create_dmg "${PRODUCT_NAME}-x86_64" "Codex-Tweaks-${RELEASE_TAG}-x86_64.dmg"

(
  cd dist
  shasum -a 256 \
    "Codex-Tweaks-${RELEASE_TAG}.dmg" \
    "Codex-Tweaks-${RELEASE_TAG}-arm64.dmg" \
    "Codex-Tweaks-${RELEASE_TAG}-x86_64.dmg" \
    > SHA256SUMS
)

echo "Release ${RELEASE_TAG} 构建完成（版本 ${MARKETING_VERSION}，构建号 ${BUILD_NUMBER}）"
