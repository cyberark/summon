#!/usr/bin/env bash
# s09-cert-auth-self-hosted.sh — S9: X.509-SVID → authn-cert → self-hosted Conjur Enterprise
#
# Obtains an X.509-SVID from the SWA Agent and authenticates to a self-hosted
# Conjur Enterprise appliance using its authn-cert authenticator. Writes a
# minimal conjurrc with environment: self-hosted to prevent the SaaS URL probe.
# Requires CONJUR_ENTERPRISE_URL; prints SKIP if unset.
#
# For a local Docker appliance provisioned by setup-swa-enterprise.sh, also
# set CONJUR_ENTERPRISE_CERT_HOST_ID, CONJUR_ENTERPRISE_CA_CERT, and
# CONJUR_ENTERPRISE_GODEBUG (all exported by sourcing /tmp/swa-e2e/enterprise-env.sh).
#
#         (beyond original 9-scenario AC table; added to validate SWA + Enterprise)

echo "Running scenario 9: X.509-SVID → self-hosted Conjur Enterprise (cert)..."

if [[ -z "$CONJUR_ENTERPRISE_URL" ]]; then
  record SKIP \
    "S9: X.509-SVID → authn-cert → self-hosted Conjur" \
    "CONJUR_ENTERPRISE_URL not set — skipped"
  return
fi

_s9_conjurrc=$(mktemp /tmp/swa-enterprise-conjurrc-XXXXXX.yml)
  printf 'account: %s\nappliance_url: %s\nenvironment: self-hosted\n' \
    "${CONJUR_ENTERPRISE_ACCOUNT}" \
    "${CONJUR_ENTERPRISE_URL}" > "${_s9_conjurrc}"

  ENTERPRISE_CERT_VARS=(
    "CONJURRC=${_s9_conjurrc}"
    "CONJUR_APPLIANCE_URL=${CONJUR_ENTERPRISE_URL}"
    "CONJUR_ACCOUNT=${CONJUR_ENTERPRISE_ACCOUNT}"
    "CONJUR_AUTHN_CERT_SERVICE_ID=${CONJUR_ENTERPRISE_CERT_SERVICE_ID}"
    "SPIFFE_ENDPOINT_SOCKET=${SPIFFE_ENDPOINT_SOCKET}"
    "CONJUR_ENVIRONMENT=self-hosted"
    # Clear the CI provisioning token and any credential paths so OIDC tokens from
    # a Conjur Cloud netrc or parent env cannot interfere with the cert auth path.
    "CONJUR_AUTHN_TOKEN="
    "NETRC_FILE="
    "CONJUR_NETRC_PATH="
  )

  # Provide explicit host identity when the appliance uses URL-path-based auth
  # (required for Enterprise when the SPIFFE URI SAN maps via san-uri annotation).
  [[ -n "${CONJUR_ENTERPRISE_CERT_HOST_ID}" ]] && \
    ENTERPRISE_CERT_VARS+=("CONJUR_AUTHN_CERT_HOST_ID=${CONJUR_ENTERPRISE_CERT_HOST_ID}")

  # Skip TLS verification for local Docker appliances with self-signed certs.
  [[ "${CONJUR_ENTERPRISE_IGNORE_SSL:-false}" == "true" ]] && \
    ENTERPRISE_CERT_VARS+=("CONJUR_IGNORE_UNTRUSTED_SSL=true")

  if [[ -n "${CONJUR_ENTERPRISE_CA_CERT}" ]]; then
    _s9_ca_tmp=$(mktemp /tmp/swa-enterprise-ca-XXXXXX.pem)
    printf '%s\n' "${CONJUR_ENTERPRISE_CA_CERT}" > "${_s9_ca_tmp}"
    ENTERPRISE_CERT_VARS+=("CONJUR_CERT_FILE=${_s9_ca_tmp}")
  fi

  # Conjur Enterprise 5.x generates DER-negative serial numbers; allow them in Go TLS.
  [[ -n "${CONJUR_ENTERPRISE_GODEBUG:-}" ]] && \
    ENTERPRISE_CERT_VARS+=("GODEBUG=${CONJUR_ENTERPRISE_GODEBUG}")

  run_summon_happy \
    "S9: X.509-SVID → authn-cert → self-hosted Conjur" \
    "${ENTERPRISE_CERT_VARS[@]}"

  [[ -n "${_s9_ca_tmp:-}" ]] && rm -f "${_s9_ca_tmp}"
  rm -f "${_s9_conjurrc}"
  unset ENTERPRISE_CERT_VARS _s9_ca_tmp _s9_conjurrc
