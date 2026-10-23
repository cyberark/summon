#!/usr/bin/env bash
# s07-cert-config-missing-files.sh — Scenario 7: Cert service ID set with no cert files
#
# When CONJUR_AUTHN_CERT_SERVICE_ID is set but CONJUR_AUTHN_CERT_FILE and
# CONJUR_AUTHN_CERT_KEY_FILE point to missing paths, the built-in provider
# returns a cert config validation error before making network calls. Also
# verifies that the test secret does not appear in the error output.
#

# ┌──────────────────────────────────────────────────────────────────────────┐
# │ Scenario 7 — CONJUR_AUTHN_CERT_SERVICE_ID set; cert or key absent        │
# └──────────────────────────────────────────────────────────────────────────┘

echo "Running scenario 7: cert service ID set, no cert files..."
run_summon_sad \
  "S7: CONJUR_AUTHN_CERT_SERVICE_ID set; cert or key absent" \
  "cannot read certificate file\|cannot read key file\|CONJUR_AUTHN_CERT_FILE\|CONJUR_AUTHN_CERT_KEY_FILE" \
  "CONJUR_APPLIANCE_URL=${CONJUR_APPLIANCE_URL}" \
  "CONJUR_ACCOUNT=${CONJUR_ACCOUNT}" \
  "CONJUR_AUTHN_CERT_SERVICE_ID=${CONJUR_AUTHN_CERT_SERVICE_ID}" \
  "CONJUR_AUTHN_CERT_FILE=/tmp/swa-e2e-nonexistent.crt" \
  "CONJUR_AUTHN_CERT_KEY_FILE=/tmp/swa-e2e-nonexistent.key" \
  "SPIFFE_ENDPOINT_SOCKET="
