#!/usr/bin/env bash
set -euo pipefail

required_variables=(
  MACOS_SIGNING_P12_BASE64
  CODE_SIGNING_EXPORT_PASSWORD
  MACOS_SIGNING_IDENTITY
  MACOS_SIGNING_CERT_SHA256
  GITHUB_ENV
)

for variable_name in "${required_variables[@]}"; do
  if [[ -z "${!variable_name:-}" ]]; then
    echo "缺少必需的发布变量：${variable_name}" >&2
    exit 1
  fi
done

runner_temp="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
signing_dir="$(mktemp -d "${runner_temp%/}/codex-tweaks-signing.XXXXXX")"
p12_path="${signing_dir}/zgccrui-macos-code-signing.p12"
certificate_path="${signing_dir}/zgccrui-macos-code-signing.pem"
keychain_path="${signing_dir}/codex-tweaks-signing.keychain-db"
keychain_password="$(openssl rand -hex 24)"

printf '%s' "$MACOS_SIGNING_P12_BASE64" \
  | tr -d '[:space:]' \
  | openssl base64 -d -A \
  > "$p12_path"
chmod 600 "$p12_path"

openssl pkcs12 \
  -in "$p12_path" \
  -clcerts \
  -nokeys \
  -passin env:CODE_SIGNING_EXPORT_PASSWORD \
  | openssl x509 -out "$certificate_path"

actual_common_name="$(
  openssl x509 -in "$certificate_path" -noout -subject -nameopt sep_multiline \
    | sed -nE 's/^[[:space:]]*CN[[:space:]]*=[[:space:]]*(.*)$/\1/p'
)"
if [[ "$actual_common_name" != "$MACOS_SIGNING_IDENTITY" ]]; then
  echo "macOS 证书名称不匹配：期望 ${MACOS_SIGNING_IDENTITY}，实际 ${actual_common_name}" >&2
  exit 1
fi

normalize_fingerprint() {
  tr '[:lower:]' '[:upper:]' | tr -cd '0-9A-F'
}

actual_sha256="$(
  openssl x509 -in "$certificate_path" -noout -fingerprint -sha256 \
    | cut -d= -f2 \
    | normalize_fingerprint
)"
expected_sha256="$(printf '%s' "$MACOS_SIGNING_CERT_SHA256" | normalize_fingerprint)"
if [[ ${#expected_sha256} -ne 64 ]] || [[ "$actual_sha256" != "$expected_sha256" ]]; then
  echo "macOS 签名证书 SHA-256 指纹不匹配。" >&2
  echo "期望：${expected_sha256:-无效值}" >&2
  echo "实际：${actual_sha256}" >&2
  exit 1
fi

certificate_sha1="$(
  openssl x509 -in "$certificate_path" -noout -fingerprint -sha1 \
    | cut -d= -f2 \
    | normalize_fingerprint
)"

security create-keychain -p "$keychain_password" "$keychain_path"
security set-keychain-settings -lut 21600 "$keychain_path"
security unlock-keychain -p "$keychain_password" "$keychain_path"
security import "$p12_path" \
  -k "$keychain_path" \
  -P "$CODE_SIGNING_EXPORT_PASSWORD" \
  -T /usr/bin/codesign \
  -T /usr/bin/security
security set-key-partition-list \
  -S apple-tool:,apple:,codesign: \
  -s \
  -k "$keychain_password" \
  "$keychain_path" \
  >/dev/null

# 自签名证书需要被 CI 的临时系统信任库接受，Xcode 才会把它视为有效
# 的 code-signing identity。GitHub 托管 macOS runner 支持无密码 sudo；
# runner 会在 job 结束后销毁，因此不会影响任何持久系统。
sudo security add-trusted-cert \
  -d \
  -r trustRoot \
  -p codeSign \
  -k /Library/Keychains/System.keychain \
  "$certificate_path"

user_keychains=()
while IFS= read -r existing_keychain; do
  existing_keychain="${existing_keychain//\"/}"
  if [[ -n "$existing_keychain" ]] && [[ "$existing_keychain" != "$keychain_path" ]]; then
    user_keychains+=("$existing_keychain")
  fi
done < <(
  security list-keychains -d user \
    | sed -E 's/^[[:space:]]*"//; s/"[[:space:]]*$//'
)
security list-keychains -d user -s "$keychain_path" "${user_keychains[@]}"

identity_is_available=false
for _ in 1 2 3 4 5; do
  if security find-identity -v -p codesigning "$keychain_path" \
    | grep -Fq "$certificate_sha1"; then
    identity_is_available=true
    break
  fi
  sleep 1
done
if [[ "$identity_is_available" != true ]]; then
  echo "已导入 macOS 证书，但没有找到可用的签名私钥。" >&2
  exit 1
fi

{
  echo "MACOS_CODE_SIGN_IDENTITY=${certificate_sha1}"
  echo "MACOS_SIGNING_KEYCHAIN=${keychain_path}"
} >> "$GITHUB_ENV"

echo "macOS 签名身份已导入并通过 SHA-256 指纹校验：${actual_sha256}"
