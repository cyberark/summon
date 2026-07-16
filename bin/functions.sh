#!/bin/bash

# Generate ephemeral test credentials so that docker-compose.yml does not
# contain hardcoded secrets.  These are single-use random values with no
# exposure outside the local test environment.
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(openssl rand -hex 16)}"
export CONJUR_DATA_KEY="${CONJUR_DATA_KEY:-$(openssl rand -base64 32)}"

function startConjur() {
  local services=(conjur postgres)

  docker compose pull "${services[@]}"
  # --wait blocks until all services pass their healthchecks (Conjur allows
  # up to 20 x 5s = 100s, which covers the time needed for DB migrations).
  docker compose up -d --wait "${services[@]}"
}

exec_on() {
  local container=$1; shift
  docker exec "$(docker compose ps -q "$container")" "$@"
}

function initEnvironment() {
  # The Conjur container is started with `server -a cucumber` (see docker-compose.yml),
  # which creates the account automatically. Retrieve the admin API key directly.
  local admin_api_key
  admin_api_key="$(exec_on conjur conjurctl role retrieve-key cucumber:user:admin)" \
    || { echo "ERROR: conjurctl role retrieve-key (admin) failed — see stderr above"; exit 1; }

  # Obtain an admin token. Run curl inside the Conjur container so that
  # 'localhost' resolves correctly without requiring published ports.
  local admin_token
  admin_token="$(exec_on conjur curl -sf --data "$admin_api_key" \
    http://localhost/authn/cucumber/admin/authenticate)" \
    || { echo "ERROR: Conjur authenticate curl failed (exit $?) — HTTP endpoint not ready"; exit 1; }
  [[ -n "$admin_token" ]] \
    || { echo "ERROR: Conjur authentication returned empty token — check admin API key"; exit 1; }

  # Load the CI test host policy via REST (inside container for same reason).
  # -o /dev/null -w "%{http_code}" captures only the HTTP status code so we can
  # fail fast with a clear message if the policy load is rejected.
  local test_host_policy
  test_host_policy='- !host
  id: ci/summon/test-host
  annotations:
    description: CI test host for summon integration tests'
  local policy_status
  policy_status="$(exec_on conjur curl -sk -o /dev/null -w "%{http_code}" -X POST \
    -H "Authorization: Token token=\"$(printf '%s' "$admin_token" | base64 -w 0)\"" \
    -H "Content-Type: text/plain" \
    --data "$test_host_policy" \
    "http://localhost/policies/cucumber/policy/root")" \
    || { echo "ERROR: policy load curl failed (exit $?) — check Conjur availability"; exit 1; }
  [[ "$policy_status" == "201" ]] \
    || { echo "ERROR: Policy load returned HTTP $policy_status (expected 201)"; exit 1; }

  # Retrieve the test host API key via conjurctl (no CLI login required)
  local test_host_api_key
  test_host_api_key="$(exec_on conjur conjurctl role retrieve-key cucumber:host:ci/summon/test-host)" \
    || { echo "ERROR: conjurctl role retrieve-key (test-host) failed — see stderr above"; exit 1; }

  # Export credentials for docker compose run test.
  # CONJUR_APPLIANCE_URL uses the docker compose service hostname, which
  # resolves inside the docker network where the test container runs.
  export CONJUR_APPLIANCE_URL="http://conjur"
  export CONJUR_AUTHN_LOGIN="host/ci/summon/test-host"
  export CONJUR_AUTHN_API_KEY="$test_host_api_key"
  export CONJUR_ADMIN_LOGIN="admin"
  export CONJUR_ADMIN_API_KEY="$admin_api_key"
}

function getAdminApiKey() {
  exec_on conjur conjurctl role retrieve-key cucumber:user:admin
}

# prepareJWT starts mock-jwt-server and exports JWT + PUBLIC_KEYS for authn-jwt tests.
# Curls from a one-off conjur container on the compose network (conjur server need not be up).
function prepareJWT() {
  docker compose up -d mock-jwt-server

  local jwt=""
  local attempts=0
  while [[ -z "$jwt" && "$attempts" -lt 30 ]]; do
    jwt="$(docker compose run -T --rm --no-deps --entrypoint curl conjur \
      -sf "http://mock-jwt-server:8080/token" 2>/dev/null | jq -r .token 2>/dev/null || true)"
    if [[ -z "$jwt" || "$jwt" == "null" ]]; then
      jwt=""
      echo "Waiting for mock JWT server to be ready..."
      sleep 1
      attempts=$((attempts + 1))
    fi
  done

  if [[ -z "$jwt" ]]; then
    echo "ERROR: mock-jwt-server did not return a token"
    exit 1
  fi

  export JWT="$jwt"

  local public_keys
  public_keys="$(docker compose run -T --rm --no-deps --entrypoint curl conjur \
    -sf "http://mock-jwt-server:8080/.well-known/jwks.json" || true)"
  if [[ -z "$public_keys" ]] || ! printf '%s' "$public_keys" | jq -e . >/dev/null 2>&1; then
    echo "ERROR: mock-jwt-server did not return valid JWKS JSON"
    exit 1
  fi
  export PUBLIC_KEYS="$public_keys"

  docker compose stop mock-jwt-server >/dev/null 2>&1 || true
}
