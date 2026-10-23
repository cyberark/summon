#!/usr/bin/env bash
# s01-jwt-svid-happy-path.sh — Scenario 1: JWT-SVID → authn-jwt → secret retrieved
#
# Exercises the full SPIFFE JWT-SVID authentication path. summon retrieves a
# secret from Conjur Cloud by authenticating with a JWT-SVID from the SWA Agent
# Workload API socket. SUMMON_PROVIDER=conjur is set globally by the entrypoint,
# which also verifies env-based provider routing.
#

# ┌──────────────────────────────────────────────────────────────────────────┐
# │ Scenario 1 — JWT-SVID → authn-jwt → secret retrieved                    │
# └──────────────────────────────────────────────────────────────────────────┘

echo "Running scenario 1: JWT-SVID happy path..."
run_summon_happy \
  "S1: JWT-SVID → authn-jwt → secret retrieved" \
  "${JWT_VARS[@]}"
