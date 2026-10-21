#!/usr/bin/env bash
# s02-cert-auth-edge.sh — Scenario 2: X.509-SVID → authn-cert → secret retrieved
#
# Exercises the X.509-SVID authentication path via Conjur Cloud Edge. summon
# retrieves a secret by presenting an X.509-SVID to the authn-cert authenticator
# on the Edge instance.
#
# CONJUR_ENVIRONMENT=self-hosted is required because Edge exposes /api paths but
# is not a SaaS tenant. Without it, conjur-api-go rejects cert auth as
# unsupported in SaaS. A minimal per-scenario conjurrc (account + appliance_url
# + environment) prevents stale OIDC tokens from the calling shell from
# interfering.
#
# Prints SKIP when SWA_EDGE_URL is unset.
#

# ┌──────────────────────────────────────────────────────────────────────────┐
# │ Scenario 2 — X.509-SVID → authn-cert → secret retrieved (Edge required) │
# └──────────────────────────────────────────────────────────────────────────┘

echo "Running scenario 2: X.509-SVID happy path (Edge)..."
if [[ -z "$SWA_EDGE_URL" ]]; then
  record SKIP \
    "S2: X.509-SVID → authn-cert → secret retrieved" \
    "SWA_EDGE_URL not set — Edge required"
  return
fi

# Use a minimal conjurrc for cert auth: no credential_storage means no
# stale OIDC token is loaded, which would conflict with cert authentication.
# Include environment: self-hosted so the library does not probe /api (Edge
# exposes /api but is self-hosted, not SaaS).
SWA_CERT_CONJURRC=$(mktemp /tmp/swa-cert-conjurrc-XXXXXX.yml)
printf 'account: %s\nappliance_url: %s\nenvironment: self-hosted\n' "$CONJUR_ACCOUNT" "$SWA_EDGE_URL" > "$SWA_CERT_CONJURRC"


# Reconstruct the workload SPIFFE ID and corresponding Conjur host path.
# setup-swa.sh registers a Conjur host whose ID equals the SPIFFE URI of the
# workload (under data/swa-e2e/). Providing the host ID in the request URL
# (request mode) is more robust against Edge versions that do not support
# SPIFFE-mode host lookup by SAN-URI annotation.
_S2_TRUST_DOMAIN="${TRUST_DOMAIN:-swa-e2e.conjur.cloud}"
_S2_NODE_GROUP="${NODE_GROUP:-swa-e2e-server-agent}"
_S2_WORKLOAD_SPIFFE_ID="spiffe://${_S2_TRUST_DOMAIN}/${_S2_NODE_GROUP}/workload/${USER}"
_S2_CERT_HOST_ID="data/swa-e2e/${_S2_WORKLOAD_SPIFFE_ID}"

CERT_EXTRA_VARS=(
  "CONJURRC=${SWA_CERT_CONJURRC}"
  "CONJUR_APPLIANCE_URL=${SWA_EDGE_URL}"
  "CONJUR_ACCOUNT=${CONJUR_ACCOUNT}"
  "CONJUR_AUTHN_CERT_SERVICE_ID=${CONJUR_AUTHN_CERT_SERVICE_ID}"
  # Provide the host ID explicitly (request mode) so Edge does not need to
  # perform a SPIFFE SAN-URI annotation lookup in the Conjur policy.
  "CONJUR_AUTHN_CERT_HOST_ID=${_S2_CERT_HOST_ID}"
  "SPIFFE_ENDPOINT_SOCKET=${SPIFFE_ENDPOINT_SOCKET}"
  # Edge serves /api but is self-hosted; skip the environment probe so that
  # cert authentication is not rejected as "unsupported in SaaS".
  "CONJUR_ENVIRONMENT=self-hosted"
  # Clear any caller-shell credential paths so OIDC tokens from a Conjur Cloud
  # netrc cannot interfere with the SPIFFE X.509-SVID cert auth path.
  "NETRC_FILE="
  "CONJUR_NETRC_PATH="
)

# Write the Edge TLS cert to a temp file for CONJUR_CERT_FILE (process
# substitution does not work when passing env vars as KEY=VALUE strings to env(1)).
EDGE_CERT_TMP=""
if [[ -n "$SWA_EDGE_CA_CERT" ]]; then
  EDGE_CERT_TMP=$(mktemp /tmp/swa-edge-cert-XXXXXX.pem)
  echo "$SWA_EDGE_CA_CERT" > "$EDGE_CERT_TMP"
  CERT_EXTRA_VARS+=("CONJUR_CERT_FILE=${EDGE_CERT_TMP}")
fi

run_summon_happy \
  "S2: X.509-SVID → authn-cert → secret retrieved" \
  "${CERT_EXTRA_VARS[@]}"

[[ -n "$EDGE_CERT_TMP" ]] && rm -f "$EDGE_CERT_TMP"
rm -f "$SWA_CERT_CONJURRC"
