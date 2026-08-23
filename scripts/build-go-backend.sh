#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
DESTINATION_DIR="${TARGET_BUILD_DIR:?}/${UNLOCALIZED_RESOURCES_FOLDER_PATH:?}"
DESTINATION="$DESTINATION_DIR/codex-tweaks-backend"
WORK_DIR="${DERIVED_FILE_DIR:?}/codex-tweaks-go-backend"

find_go() {
  local candidate
  if command -v go >/dev/null 2>&1; then
    command -v go
    return
  fi
  for candidate in \
    "$HOME/.local/share/mise/shims/go" \
    /opt/homebrew/bin/go \
    /usr/local/bin/go; do
    if [[ -x "$candidate" ]]; then
      echo "$candidate"
      return
    fi
  done
  echo "找不到 Go 工具链；请先运行 mise install。" >&2
  exit 1
}

GO_BIN="$(find_go)"
read -r -a XCODE_ARCHS <<< "${ARCHS:-${CURRENT_ARCH:-}}"
if [[ ${#XCODE_ARCHS[@]} -eq 0 ]]; then
  echo "Xcode 未提供目标架构。" >&2
  exit 1
fi

rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR" "$DESTINATION_DIR"

OUTPUTS=()
for xcode_arch in "${XCODE_ARCHS[@]}"; do
  case "$xcode_arch" in
    arm64) go_arch="arm64" ;;
    x86_64) go_arch="amd64" ;;
    *)
      echo "不支持的 Go 后端架构：$xcode_arch" >&2
      exit 1
      ;;
  esac

  output="$WORK_DIR/codex-tweaks-backend-$xcode_arch"
  release_version="${CODEX_TWEAKS_RELEASE_VERSION:-${MARKETING_VERSION:-dev}}"
  (
    cd "$BACKEND_DIR"
    CGO_ENABLED=0 GOOS=darwin GOARCH="$go_arch" "$GO_BIN" build \
      -trimpath \
      -ldflags "-s -w -X main.version=$release_version" \
      -o "$output" \
      ./cmd/codex-tweaks-backend
  )
  OUTPUTS+=("$output")
done

if [[ ${#OUTPUTS[@]} -eq 1 ]]; then
  cp "${OUTPUTS[0]}" "$DESTINATION"
else
  /usr/bin/lipo -create "${OUTPUTS[@]}" -output "$DESTINATION"
fi

chmod 755 "$DESTINATION"
/usr/bin/codesign --force --sign "${EXPANDED_CODE_SIGN_IDENTITY:--}" --timestamp=none "$DESTINATION"
