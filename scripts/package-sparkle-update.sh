#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

RELEASE_TAG="${RELEASE_TAG:-${1:-}}"
BUILD_NUMBER="${BUILD_NUMBER:-1}"
DIST_DIR="${DIST_DIR:-dist}"
GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-codex-tweaks/codex-tweaks}"
SPARKLE_EDDSA_PRIVATE_KEY="${SPARKLE_EDDSA_PRIVATE_KEY:-}"
SPARKLE_PUBLIC_ED_KEY="${SPARKLE_PUBLIC_ED_KEY:-}"

if [[ ! "$RELEASE_TAG" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)(-[0-9A-Za-z][0-9A-Za-z.-]*)?$ ]]; then
  echo "RELEASE_TAG 必须是 v1.2.3 或 v1.2.3-beta.1 形式" >&2
  exit 1
fi
expected_marketing_version="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.${BASH_REMATCH[3]}"
if [[ ! "$BUILD_NUMBER" =~ ^[1-9][0-9]*$ ]]; then
  echo "BUILD_NUMBER 必须是正整数" >&2
  exit 1
fi
if [[ -z "$SPARKLE_EDDSA_PRIVATE_KEY" ]]; then
  echo "缺少 SPARKLE_EDDSA_PRIVATE_KEY，无法签名 Sparkle 更新。" >&2
  exit 1
fi
if [[ -z "$SPARKLE_PUBLIC_ED_KEY" ]]; then
  echo "缺少 SPARKLE_PUBLIC_ED_KEY，无法校验 App 内置公钥。" >&2
  exit 1
fi

derived_public_key="$(
  swift - <<'SWIFT'
import CryptoKit
import Foundation

let encodedPrivateKey = ProcessInfo.processInfo.environment["SPARKLE_EDDSA_PRIVATE_KEY"] ?? ""
guard let privateSeed = Data(base64Encoded: encodedPrivateKey), privateSeed.count == 32 else {
    fputs("Sparkle 私钥必须是 Base64 编码的 32 字节 Ed25519 seed。\n", stderr)
    exit(1)
}
let privateKey = try Curve25519.Signing.PrivateKey(rawRepresentation: privateSeed)
print(privateKey.publicKey.rawRepresentation.base64EncodedString())
SWIFT
)"
if [[ "$derived_public_key" != "$SPARKLE_PUBLIC_ED_KEY" ]]; then
  echo "Sparkle 私钥与 GitHub Variable 中的公钥不属于同一密钥对。" >&2
  exit 1
fi

product_name="Codex Tweaks"
app_path="${DIST_DIR}/${product_name}.app"
archive_name="Codex-Tweaks-${RELEASE_TAG}-sparkle.zip"
archive_path="${DIST_DIR}/${archive_name}"
appcast_path="${DIST_DIR}/appcast.xml"

if [[ ! -d "$app_path" ]]; then
  echo "缺少 universal App：${app_path}" >&2
  exit 1
fi

main_binary="$app_path/Contents/MacOS/${product_name}"
backend_binary="$app_path/Contents/Resources/codex-tweaks-backend"
for binary_path in "$main_binary" "$backend_binary"; do
  actual_architectures="$(lipo -archs "$binary_path" | tr ' ' '\n' | sort | xargs)"
  if [[ "$actual_architectures" != "arm64 x86_64" ]]; then
    echo "Sparkle 更新必须包含 universal App；${binary_path} 实际为 ${actual_architectures}。" >&2
    exit 1
  fi
done

expected_release_version="${RELEASE_TAG#v}"
actual_release_version="$(
  /usr/libexec/PlistBuddy -c 'Print :CodexTweaksReleaseVersion' "$app_path/Contents/Info.plist"
)"
actual_marketing_version="$(
  /usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$app_path/Contents/Info.plist"
)"
actual_build_number="$(
  /usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$app_path/Contents/Info.plist"
)"
if [[ "$actual_release_version" != "$expected_release_version" ]] \
  || [[ "$actual_marketing_version" != "$expected_marketing_version" ]] \
  || [[ "$actual_build_number" != "$BUILD_NUMBER" ]]; then
  echo "Sparkle App 版本错误：期望 ${expected_release_version} / ${expected_marketing_version} (${BUILD_NUMBER})，实际 ${actual_release_version} / ${actual_marketing_version} (${actual_build_number})" >&2
  exit 1
fi

built_public_key="$(
  /usr/libexec/PlistBuddy -c 'Print :SUPublicEDKey' "$app_path/Contents/Info.plist"
)"
if [[ "$built_public_key" != "$SPARKLE_PUBLIC_ED_KEY" ]]; then
  echo "App 内置 Sparkle 公钥与 GitHub Variable 不一致。" >&2
  exit 1
fi

feed_url="$(
  /usr/libexec/PlistBuddy -c 'Print :SUFeedURL' "$app_path/Contents/Info.plist"
)"
expected_feed_url="https://raw.githubusercontent.com/${GITHUB_REPOSITORY}/updates/appcast.xml"
if [[ "$feed_url" != "$expected_feed_url" ]]; then
  echo "Sparkle feed 地址错误：期望 ${expected_feed_url}，实际 ${feed_url}" >&2
  exit 1
fi

codesign --verify --deep --strict --verbose=2 "$app_path"
unlink "$archive_path" 2>/dev/null || true
ditto -c -k --sequesterRsrc --keepParent "$app_path" "$archive_path"

staging_dir="$(mktemp -d "${TMPDIR:-/tmp}/codex-tweaks-appcast.XXXXXX")"
cleanup() {
  find "$staging_dir" -depth -delete 2>/dev/null || true
}
trap cleanup EXIT

cp "$archive_path" "$staging_dir/$archive_name"
cache_buster="${GITHUB_RUN_ID:-$(date +%s)}"
existing_appcast_url="https://raw.githubusercontent.com/${GITHUB_REPOSITORY}/updates/appcast.xml?source=${cache_buster}"
http_status="$(
  curl \
    --silent \
    --show-error \
    --location \
    --output "$staging_dir/appcast.xml" \
    --write-out '%{http_code}' \
    "$existing_appcast_url"
)"
case "$http_status" in
  200)
    echo "已载入现有 Sparkle appcast，将保留历史更新记录。"
    ;;
  404)
    unlink "$staging_dir/appcast.xml" 2>/dev/null || true
    echo "未发现现有 Sparkle appcast，将创建第一版 feed。"
    ;;
  *)
    echo "读取现有 Sparkle appcast 失败（HTTP ${http_status}）。" >&2
    exit 1
    ;;
esac

generate_arguments=(
  --ed-key-file -
  --download-url-prefix "https://github.com/${GITHUB_REPOSITORY}/releases/download/${RELEASE_TAG}/"
  --link "https://github.com/${GITHUB_REPOSITORY}/releases/tag/${RELEASE_TAG}"
  --versions "$BUILD_NUMBER"
  --maximum-versions 10
  --maximum-deltas 0
  --disable-signing-warning
)
if [[ "$RELEASE_TAG" == *-* ]]; then
  generate_arguments+=(--channel beta)
fi
generate_arguments+=("$staging_dir")

printf '%s\n' "$SPARKLE_EDDSA_PRIVATE_KEY" \
  | generate_appcast "${generate_arguments[@]}"
printf '%s\n' "$SPARKLE_EDDSA_PRIVATE_KEY" \
  | sign_update --verify --ed-key-file - "$staging_dir/appcast.xml"

cp "$staging_dir/appcast.xml" "$appcast_path"
echo "已创建并验证 Sparkle 更新：${archive_path} 与 ${appcast_path}"
