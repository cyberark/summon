#!/usr/bin/env bash
# setup-lib.sh — Shared helpers for SWA setup scripts.
#
# Source this file from setup scripts:
#   source "$(dirname "${BASH_SOURCE[0]}")/swa/setup-lib.sh"

# urlencode <string>
# URL-encodes a string using RFC 3986 reserved-character rules.
urlencode() {
  local s="${1}" enc="" c
  for (( i = 0; i < ${#s}; i++ )); do
    c="${s:i:1}"
    case "$c" in
      [a-zA-Z0-9.~_-]) enc+="$c" ;;
      *) enc+=$(printf '%%%02X' "'$c") ;;
    esac
  done
  printf '%s' "$enc"
}

# base64url
# Reads stdin and writes base64url output (RFC 4648 §5, no padding).
base64url() {
  base64 | tr '+/' '-_' | tr -d '=\n'
}

# appliance_url_from_conjurrc <conjurrc_file>
# Extracts the appliance_url value from a conjurrc YAML file.
appliance_url_from_conjurrc() {
  local file="$1" url
  url=$(awk '/^[[:space:]]*appliance_url:/{print $2; exit}' "$file" \
    | tr -d '"'"'" | sed 's|/$||')
  printf '%s' "$url"
}

# oidc_token_b64 <netrc_file>
# Reads the OIDC password from a conjur netrc file and prints its base64 encoding.
# Exits non-zero and prints nothing if the token is not found.
oidc_token_b64() {
  local file="$1" raw
  raw=$(awk '/login \[oidc\]/{found=1} found && $1=="password"{print $2; found=0; exit}' "$file")
  [[ -z "$raw" ]] && return 1
  printf '%s' "$raw" | base64 | tr -d '\n'
}
