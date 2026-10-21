#!/usr/bin/env bash
# s03-enoent-socket.sh — Scenario 3: SPIFFE_ENDPOINT_SOCKET → non-existent path
#
# When SPIFFE_ENDPOINT_SOCKET points to a non-existent path, the error message
# contains "not found", "no such file", or "ENOENT". Also verifies that the
# test secret does not appear in the error output.
#

# ┌──────────────────────────────────────────────────────────────────────────┐
# │ Scenario 3 — SPIFFE_ENDPOINT_SOCKET → non-existent path (ENOENT)        │
# └──────────────────────────────────────────────────────────────────────────┘

echo "Running scenario 3: non-existent socket path..."
run_summon_sad \
  "S3: SPIFFE_ENDPOINT_SOCKET → non-existent path" \
  "not found\|no such file\|ENOENT" \
  "CONJUR_APPLIANCE_URL=${CONJUR_APPLIANCE_URL}" \
  "CONJUR_ACCOUNT=${CONJUR_ACCOUNT}" \
  "CONJUR_AUTHN_JWT_SERVICE_ID=${CONJUR_AUTHN_JWT_SERVICE_ID}" \
  "SPIFFE_ENDPOINT_SOCKET=unix:///tmp/swa-e2e-does-not-exist.sock" \
  "CONJUR_AUTHN_TOKEN="
