#!/bin/bash -e
# Boots Conjur Enterprise and generates client certificates for authn-cert tests.
# Source from bin/test.sh when TEST_CERT=true (exports CERT_TMPDIR and cert env vars).

cd "$(dirname "${BASH_SOURCE[0]}")/.."

announce() {
  echo "
  ================================
   ${1}
  ================================
  "
}

export CONJUR_ADMIN_PASSWORD="${CONJUR_ADMIN_PASSWORD:-MySecretP@ss1}"

function start_conjur_appliance() {
  announce "Pulling Conjur Enterprise appliance image..."
  docker compose --profile cert pull conjur-leader

  announce "Starting Conjur Enterprise appliance..."
  docker compose --profile cert up --no-deps -d conjur-leader

  local timeout=60
  local elapsed=0
  until docker compose --profile cert exec -T conjur-leader true 2>/dev/null; do
    if (( elapsed >= timeout )); then
      echo "Timed out waiting for conjur-leader container to be exec-able"
      exit 1
    fi
    sleep 2
    (( elapsed += 2 ))
  done

  announce "Configuring Conjur Enterprise master..."
  timeout=120
  elapsed=0
  until docker compose --profile cert exec -T conjur-leader evoke configure master \
    --hostname=conjur-leader-1.mycompany.local \
    --master-altnames="conjur-leader.mycompany.local,conjur-leader-1.mycompany.local" \
    --accept-eula \
    --admin-password="${CONJUR_ADMIN_PASSWORD}" \
    conjur; do
    if (( elapsed >= timeout )); then
      echo "Timed out configuring Conjur Enterprise master"
      exit 1
    fi
    sleep 5
    (( elapsed += 5 ))
  done

  export CONJUR_CERT_AUTHN_LOGIN="${CONJUR_CERT_AUTHN_LOGIN:-admin}"

  announce "Waiting for enterprise appliance auth endpoint..."
  timeout=120
  elapsed=0
  until docker compose --profile cert exec -T conjur-leader test -f /opt/conjur/etc/ssl/conjur.pem; do
    if (( elapsed >= timeout )); then
      echo "Timed out waiting for enterprise appliance TLS certificate"
      exit 1
    fi
    sleep 2
    (( elapsed += 2 ))
  done

  CONJUR_CERT_AUTHN_API_KEY="$(docker compose --profile cert exec -T conjur-leader \
    curl -sfk --user "${CONJUR_CERT_AUTHN_LOGIN}:${CONJUR_ADMIN_PASSWORD}" \
    https://localhost/authn/conjur/login | tr -d '\r\n')"
  if [[ -z "${CONJUR_CERT_AUTHN_API_KEY}" || "${CONJUR_CERT_AUTHN_API_KEY}" == *"<"* ]]; then
    echo "ERROR: failed to retrieve admin API key from enterprise appliance"
    exit 1
  fi
  export CONJUR_CERT_AUTHN_API_KEY

  CONJUR_CERT_SSL_CERTIFICATE="$(docker compose --profile cert exec -T conjur-leader \
    cat /opt/conjur/etc/ssl/conjur.pem)"
  export CONJUR_CERT_SSL_CERTIFICATE
}

function generate_cert_auth_pki() {
  announce "Generating CA and client certificates for authn-cert tests..."

  export CERT_TMPDIR
  CERT_TMPDIR="$(mktemp -d)"

  openssl genrsa -out "$CERT_TMPDIR/ca.key" 4096 2>/dev/null
  openssl req -new -x509 -days 3650 \
    -key "$CERT_TMPDIR/ca.key" \
    -out "$CERT_TMPDIR/ca.pem" \
    -subj "/C=US/O=ConjurTestCA/CN=Conjur Test Certificate Authority"

  openssl genrsa -out "$CERT_TMPDIR/client.key" 2048 2>/dev/null
  openssl req -new \
    -key "$CERT_TMPDIR/client.key" \
    -out "$CERT_TMPDIR/client.csr" \
    -subj "/C=US/O=ConjurTest/CN=vm-01"

  cat > "$CERT_TMPDIR/client.ext" <<EOF
subjectAltName=URI:spiffe://conjur.test/vm-spiffe
extendedKeyUsage=clientAuth
EOF

  openssl x509 -req -days 365 \
    -in "$CERT_TMPDIR/client.csr" \
    -CA "$CERT_TMPDIR/ca.pem" \
    -CAkey "$CERT_TMPDIR/ca.key" \
    -CAcreateserial \
    -extfile "$CERT_TMPDIR/client.ext" \
    -out "$CERT_TMPDIR/client.pem" 2>/dev/null

  export CONJUR_AUTHN_CERT_FILE="/certs/client.pem"
  export CONJUR_AUTHN_CERT_KEY_FILE="/certs/client.key"
  export TEST_CERT_CA_CERT
  TEST_CERT_CA_CERT="$(cat "$CERT_TMPDIR/ca.pem")"
}

start_conjur_appliance
generate_cert_auth_pki

export CONJUR_CERT_APPLIANCE_URL="https://conjur-leader-1.mycompany.local"

announce "Certificate auth test environment ready."
echo "  Appliance URL : $CONJUR_CERT_APPLIANCE_URL"
echo "  Cert dir      : $CERT_TMPDIR"
