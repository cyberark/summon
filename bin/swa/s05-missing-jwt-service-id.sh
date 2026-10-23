#!/usr/bin/env bash
# s05-missing-jwt-service-id.sh — Scenario 5: CONJUR_AUTHN_JWT_SERVICE_ID absent
#
# When CONJUR_AUTHN_JWT_SERVICE_ID is not set, the built-in provider returns a
# pre-flight validation error before any network call is made. The error message
# names the missing variable. Also verifies that the test secret does not appear
# in the error output.
#

# ┌──────────────────────────────────────────────────────────────────────────┐
# │ Scenario 5 — CONJUR_AUTHN_JWT_SERVICE_ID absent → config error           │
# └──────────────────────────────────────────────────────────────────────────┘

# CONJUR_AUTHN_JWT_SERVICE_ID is intentionally omitted from the env list so the
# built-in provider hits its "Must specify a ServiceID" validation.
echo "Running scenario 5: CONJUR_AUTHN_JWT_SERVICE_ID absent..."
run_summon_sad \
  "S5: CONJUR_AUTHN_JWT_SERVICE_ID absent → Must specify a ServiceID" \
  "SPIFFE_ENDPOINT_SOCKET is set but no authenticator service ID is configured\|CONJUR_AUTHN_JWT_SERVICE_ID\|CONJUR_AUTHN_CERT_SERVICE_ID" \
  --clean \
  "CONJUR_APPLIANCE_URL=${CONJUR_APPLIANCE_URL}" \
  "CONJUR_ACCOUNT=${CONJUR_ACCOUNT}" \
  "SPIFFE_ENDPOINT_SOCKET=${SPIFFE_ENDPOINT_SOCKET}"
