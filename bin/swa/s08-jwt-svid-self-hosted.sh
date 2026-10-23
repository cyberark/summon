#!/usr/bin/env bash
# s08-jwt-svid-self-hosted.sh — S8: JWT-SVID → authn-jwt → self-hosted Conjur Enterprise
#
# Obtains a JWT-SVID from the SWA Agent and authenticates to a self-hosted
# Conjur Enterprise appliance using its authn-jwt authenticator.
# Requires CONJUR_ENTERPRISE_URL; prints SKIP if unset.
#
# For a local Docker appliance provisioned by setup-swa-enterprise.sh, also
# set CONJUR_ENTERPRISE_JWT_HOST_ID, CONJUR_ENTERPRISE_CA_CERT, and
# CONJUR_ENTERPRISE_GODEBUG (all exported by sourcing /tmp/swa-e2e/enterprise-env.sh).
#
#         (beyond original 9-scenario AC table; added to validate SWA + Enterprise)

echo "Running scenario 8: JWT-SVID → self-hosted Conjur Enterprise (JWT)..."

if [[ -z "$CONJUR_ENTERPRISE_URL" ]]; then
  record SKIP \
    "S8: JWT-SVID → authn-jwt → self-hosted Conjur" \
    "CONJUR_ENTERPRISE_URL not set — skipped"
  return
fi

ENTERPRISE_JWT_VARS=(
    "CONJUR_APPLIANCE_URL=${CONJUR_ENTERPRISE_URL}"
    "CONJUR_ACCOUNT=${CONJUR_ENTERPRISE_ACCOUNT}"
    "CONJUR_AUTHN_JWT_SERVICE_ID=${CONJUR_ENTERPRISE_JWT_SERVICE_ID}"
    "SPIFFE_ENDPOINT_SOCKET=${SPIFFE_ENDPOINT_SOCKET}"
    "CONJUR_ENVIRONMENT=self-hosted"
    # Clear the CI provisioning token so summon takes the SPIFFE path.
    "CONJUR_AUTHN_TOKEN="
  )

  # Provide explicit host identity when the appliance uses URL-path-based auth
  # (required for Enterprise when SPIFFE sub claim contains "://").
  [[ -n "${CONJUR_ENTERPRISE_JWT_HOST_ID}" ]] && \
    ENTERPRISE_JWT_VARS+=("CONJUR_AUTHN_JWT_HOST_ID=${CONJUR_ENTERPRISE_JWT_HOST_ID}")

  # Skip TLS verification for local Docker appliances with self-signed certs.
  [[ "${CONJUR_ENTERPRISE_IGNORE_SSL:-false}" == "true" ]] && \
    ENTERPRISE_JWT_VARS+=("CONJUR_IGNORE_UNTRUSTED_SSL=true")

  if [[ -n "${CONJUR_ENTERPRISE_CA_CERT}" ]]; then
    _s8_ca_tmp=$(mktemp /tmp/swa-enterprise-ca-XXXXXX.pem)
    printf '%s\n' "${CONJUR_ENTERPRISE_CA_CERT}" > "${_s8_ca_tmp}"
    ENTERPRISE_JWT_VARS+=("CONJUR_CERT_FILE=${_s8_ca_tmp}")
  fi

  # Conjur Enterprise 5.x generates DER-negative serial numbers; allow them in Go TLS.
  [[ -n "${CONJUR_ENTERPRISE_GODEBUG:-}" ]] && \
    ENTERPRISE_JWT_VARS+=("GODEBUG=${CONJUR_ENTERPRISE_GODEBUG}")

  run_summon_happy \
    "S8: JWT-SVID → authn-jwt → self-hosted Conjur" \
    "${ENTERPRISE_JWT_VARS[@]}"

  [[ -n "${_s8_ca_tmp:-}" ]] && rm -f "${_s8_ca_tmp}"
  unset ENTERPRISE_JWT_VARS _s8_ca_tmp
