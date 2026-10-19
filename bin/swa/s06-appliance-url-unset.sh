#!/usr/bin/env bash
# s06-appliance-url-unset.sh — Scenario 6: CONJUR_APPLIANCE_URL unset
#
# Passing CONJUR_APPLIANCE_URL= (empty) alongside an empty CONJURRC triggers
# the "Must specify an ApplianceURL" validation error before any network call
# is made. Also verifies that the test secret does not appear in the error
# output.
#
# Covers: CNJR-15383 scenario #8 (CONJUR_APPLIANCE_URL unset → config error)

# ┌──────────────────────────────────────────────────────────────────────────┐
# │ Scenario 6 — CONJUR_APPLIANCE_URL unset                                  │
# └──────────────────────────────────────────────────────────────────────────┘

# Pass CONJUR_APPLIANCE_URL= (empty) to override the parent shell's value.
# Use an empty CONJURRC so no URL is loaded from file either.
# Both are needed: env mergeValue skips empty env vars, so the conjurrc fallback
# must also be cleared to get the "Must specify an ApplianceURL" validation error.
echo "Running scenario 6: CONJUR_APPLIANCE_URL unset..."
: > /tmp/swa-e2e-empty-conjurrc.yml
run_summon_sad \
  "S6: CONJUR_APPLIANCE_URL unset → validation error" \
  "ApplianceURL\|appliance" \
  "CONJUR_APPLIANCE_URL=" \
  "CONJUR_ACCOUNT=${CONJUR_ACCOUNT}" \
  "CONJUR_AUTHN_JWT_SERVICE_ID=${CONJUR_AUTHN_JWT_SERVICE_ID}" \
  "SPIFFE_ENDPOINT_SOCKET=${SPIFFE_ENDPOINT_SOCKET}" \
  "CONJURRC=/tmp/swa-e2e-empty-conjurrc.yml"
