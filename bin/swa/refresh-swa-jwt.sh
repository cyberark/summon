#!/usr/bin/env bash
# refresh-swa-jwt.sh — Regenerate the SWA Server authentication JWT.
#
# The SWA Server reads its JWT from tokenPath at every syncInterval (default 5m).
# The token is signed with the RSA-2048 key generated during initial provisioning
# and must carry the same sub/iss/aud values registered with Conjur Cloud.
#
# Run before starting the SWA Server and any time the token nears expiry.
#
# Usage:
#   bin/swa/refresh-swa-jwt.sh
#
# Environment:
#   SWA_WORKDIR   Working directory containing PKI and provision-server.json (default: /tmp/swa-e2e)
#   JWT_TTL_SEC   Token lifetime in seconds (default: 3600)

set -euo pipefail

# ---------------------------------------------------------------------------


main() {
  setup_env
  check_prerequisites
  load_jwt_claims
  build_jwt
  print_summary
}

setup_env() {
  SWA_WORKDIR="${SWA_WORKDIR:-/tmp/swa-e2e}"
  JWT_TTL_SEC="${JWT_TTL_SEC:-3600}"

  KEY="${SWA_WORKDIR}/server-jwt.key"
  OUT="${SWA_WORKDIR}/server.jwt"
}

check_prerequisites() {
  if [[ ! -f "$KEY" ]]; then
    echo "ERROR: ${KEY} not found. Run bin/swa/setup-swa.sh first." >&2
    exit 1
  fi
}

load_jwt_claims() {
  # Read registered sub/iss/aud from provision-server.json written by setup-swa.sh.
  PROVISION_JSON="${SWA_WORKDIR}/provision-server.json"
  if [[ -f "$PROVISION_JSON" ]]; then
    SUB=$(jq -r '.authentication.data.sub    // "swa-e2e-server"'   "$PROVISION_JSON")
    ISS=$(jq -r '.authentication.data.issuer // "https://swa-e2e.local"' "$PROVISION_JSON")
    AUD=$(jq -r '.authentication.data.audience // "conjur"'          "$PROVISION_JSON")
  else
    SUB="${SERVER_JWT_SUB:-swa-e2e-server}"
    ISS="${SERVER_JWT_ISS:-https://swa-e2e.local}"
    AUD="${SERVER_JWT_AUD:-conjur}"
  fi
}

build_jwt() {
  NOW=$(date +%s)
  EXP=$(( NOW + JWT_TTL_SEC ))

  # Build base64url-encoded header and payload.
  _HEADER=$(printf '{"alg":"RS256","kid":"swa-e2e-key-1","typ":"JWT"}' \
    | base64 | tr '+/' '-_' | tr -d '=\n')
  _PAYLOAD=$(jq -ncj \
    --arg sub "$SUB" --arg iss "$ISS" --arg aud "$AUD" \
    --argjson iat "$NOW" --argjson exp "$EXP" \
    '{sub:$sub,iss:$iss,aud:$aud,iat:$iat,exp:$exp}' \
    | base64 | tr '+/' '-_' | tr -d '=\n')

  _SIGNING_INPUT="${_HEADER}.${_PAYLOAD}"

  # Sign with RSA-SHA256 via openssl, then base64url-encode the DER signature.
  _SIG=$(printf '%s' "$_SIGNING_INPUT" \
    | openssl dgst -sha256 -sign "$KEY" -binary \
    | base64 | tr '+/' '-_' | tr -d '=\n')

  JWT="${_SIGNING_INPUT}.${_SIG}"
  printf '%s' "$JWT" > "$OUT"
}

print_summary() {
  # Display summary.
  REMAINING=$(( EXP - $(date +%s) ))
  echo "JWT written to ${OUT}"
  echo "  sub=${SUB}  iss=${ISS}"
  echo "  valid for ${REMAINING}s (expires epoch ${EXP})"
}

main "$@"
