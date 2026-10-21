#!/usr/bin/env bash
# s06-appliance-url-unset.sh — Scenario 6: cert auth over HTTP → HTTPS required
#
# Verifies that summon emits a config-time error (not a secret leak) when cert
# authentication is requested but the appliance URL is HTTP, not HTTPS. conjur-
# api-go requires HTTPS for cert auth and returns the error before any network
# call is made.
#
# A CONJURRC with an explicit HTTP appliance URL overrides any system-level
# /etc/conjur.conf so the validation fires consistently in CI environments that
# have a pre-configured system conjurrc.
#

# ┌──────────────────────────────────────────────────────────────────────────┐
# │ Scenario 6 — cert auth over HTTP URL → validation error                  │
# └──────────────────────────────────────────────────────────────────────────┘

echo "Running scenario 6: cert auth over HTTP → HTTPS required error..."

# Write a CONJURRC with an HTTP appliance URL. The non-empty value overrides
# any HTTPS URL that may be present in the system /etc/conjur.conf, so the
# "Certificate authentication requires an HTTPS connection" Validate() check
# fires before any network call. CONJUR_APPLIANCE_URL echoes the same HTTP
# URL so mergeEnv does not accidentally restore the system URL.
_S6_CONJURRC=$(mktemp /tmp/swa-e2e-s6-conjurrc-XXXXXX.yml)
printf 'account: %s\nappliance_url: http://conjur.invalid.example.com\n' \
    "${CONJUR_ACCOUNT}" > "${_S6_CONJURRC}"

run_summon_sad \
  "S6: cert auth → HTTPS connection required" \
  "HTTPS\|Certificate authentication requires" \
  --clean \
  "CONJUR_ACCOUNT=${CONJUR_ACCOUNT}" \
  "CONJURRC=${_S6_CONJURRC}" \
  "CONJUR_APPLIANCE_URL=http://conjur.invalid.example.com" \
  "CONJUR_AUTHN_CERT_SERVICE_ID=${CONJUR_AUTHN_CERT_SERVICE_ID}" \
  "SPIFFE_ENDPOINT_SOCKET=${SPIFFE_ENDPOINT_SOCKET}" \
  "SUMMON_PROVIDER=conjur"

rm -f "${_S6_CONJURRC}"
