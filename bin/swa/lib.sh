#!/usr/bin/env bash
# lib.sh — Shared helpers for the SWA E2E scenario runner.
#
# Sourced by run-swa-scenarios.sh before any scenario files are loaded.
# Expects the following variables to already be set by the caller:
#   EXPECTED_SECRET  TEST_VAR_PATH  SUMMON_BINARY

# ── Pass/fail tracking ──────────────────────────────────────────────────────

PASS=0
FAIL=0
SKIP=0
declare -a RESULTS=()

# ── Standard env-var bundles for happy-path scenarios ───────────────────────
#
# JWT_VARS: minimum env for a SPIFFE JWT-SVID → authn-jwt call.
# Scenarios source lib.sh after run-swa-scenarios.sh calls setup_env(), so all
# these variables are already resolved at the time the array is built.
# shellcheck disable=SC2034  # used by s01-jwt-svid-happy-path.sh and other sourcing scripts
JWT_VARS=(
  "CONJUR_APPLIANCE_URL=${CONJUR_APPLIANCE_URL}"
  "CONJUR_ACCOUNT=${CONJUR_ACCOUNT}"
  "CONJUR_AUTHN_JWT_SERVICE_ID=${CONJUR_AUTHN_JWT_SERVICE_ID}"
  "SPIFFE_ENDPOINT_SOCKET=${SPIFFE_ENDPOINT_SOCKET}"
  # Clear the CI provisioning token so summon takes the SPIFFE path, not the
  # token-auth path.  By the time scenarios run, the token has been used for
  # provisioning and must not leak into scenario invocations.
  "CONJUR_AUTHN_TOKEN="
)

record() {
  local status="$1" label="$2" detail="${3:-}"
  RESULTS+=("${status}|${label}|${detail}")
  case "$status" in
    PASS) PASS=$((PASS + 1)) ;;
    FAIL) FAIL=$((FAIL + 1)) ;;
    SKIP) SKIP=$((SKIP + 1)) ;;
  esac
}

# ── Scenario helpers ────────────────────────────────────────────────────────
#
# These helpers call `env VAR=val ... summon ...` directly.
# Callers pass KEY=VALUE pairs as positional args, then optionally
# "--extra" followed by additional summon flags.
#
# run_summon_happy LABEL [KEY=VAL...] [--extra SUMMON_FLAG...]
# run_summon_sad   LABEL EXPECTED_ERR_SUBSTRING [KEY=VAL...] [--extra SUMMON_FLAG...]

run_summon_happy() {
  local label="$1"; shift
  local yaml_spec="RETRIEVED: !var ${TEST_VAR_PATH}"
  local env_args=()
  local summon_extra=()
  local after_sep=false

  for arg in "$@"; do
    if [[ "$after_sep" == "true" ]]; then
      summon_extra+=("$arg")
    elif [[ "$arg" == "--extra" ]]; then
      after_sep=true
    else
      env_args+=("$arg")
    fi
  done

  local tmpout tmperr rc
  tmpout=$(mktemp); tmperr=$(mktemp)

  set +e
  env "${env_args[@]+"${env_args[@]}"}" \
    "${SUMMON_BINARY}" "${summon_extra[@]+"${summon_extra[@]}"}" \
    --yaml "$yaml_spec" \
    -- printenv RETRIEVED >"$tmpout" 2>"$tmperr"
  rc=$?
  set -e

  local stdout stderr
  stdout=$(cat "$tmpout"); rm -f "$tmpout"
  stderr=$(cat "$tmperr"); rm -f "$tmperr"

  if [[ $rc -ne 0 ]]; then
    record FAIL "$label" "summon exited ${rc}: $(echo "$stderr" | head -1 | cut -c1-80)"
    return
  fi
  if ! echo "$stdout" | grep -qF "$EXPECTED_SECRET"; then
    record FAIL "$label" "secret value not in output (got: $(echo "$stdout" | head -1 | cut -c1-60))"
    return
  fi
  record PASS "$label" "secret retrieved"
}

run_summon_sad() {
  local label="$1" expected_err="$2"; shift 2
  local yaml_spec="RETRIEVED: !var ${TEST_VAR_PATH}"
  local env_args=()
  local summon_extra=()
  local after_sep=false
  local clean_env=false

  for arg in "$@"; do
    if [[ "$after_sep" == "true" ]]; then
      summon_extra+=("$arg")
    elif [[ "$arg" == "--extra" ]]; then
      after_sep=true
    elif [[ "$arg" == "--clean" ]]; then
      # --clean: run summon with a minimal environment (only PATH + explicit vars).
      # Use this when the parent process exports auth vars that should be absent.
      clean_env=true
    else
      env_args+=("$arg")
    fi
  done

  local tmpout tmperr rc
  tmpout=$(mktemp); tmperr=$(mktemp)

  set +e
  if [[ "$clean_env" == "true" ]]; then
    env -i PATH="${PATH}" \
      HOME="/tmp" \
      SUMMON_PROVIDER="${SUMMON_PROVIDER:-conjur}" \
      "${env_args[@]+"${env_args[@]}"}" \
      "${SUMMON_BINARY}" "${summon_extra[@]+"${summon_extra[@]}"}" \
      --yaml "$yaml_spec" \
      -- printenv RETRIEVED >"$tmpout" 2>"$tmperr"
  else
    env "${env_args[@]+"${env_args[@]}"}" \
      "${SUMMON_BINARY}" "${summon_extra[@]+"${summon_extra[@]}"}" \
      --yaml "$yaml_spec" \
      -- printenv RETRIEVED >"$tmpout" 2>"$tmperr"
  fi
  rc=$?
  set -e

  local stdout stderr
  stdout=$(cat "$tmpout"); rm -f "$tmpout"
  stderr=$(cat "$tmperr"); rm -f "$tmperr"

  if [[ $rc -eq 0 ]]; then
    record FAIL "$label" "expected summon to fail but it exited 0"
    return
  fi
  if [[ -n "$expected_err" ]] && ! echo "$stderr" | grep -qi "$expected_err"; then
    record FAIL "$label" "expected '${expected_err}' in stderr, got: $(echo "$stderr" | head -1 | cut -c1-80)"
    return
  fi
  # Secret leakage check: the test secret value must not appear in any error output.
  if echo "$stderr" | grep -qF "$EXPECTED_SECRET"; then
    record FAIL "$label" "secret value appeared in error output"
    return
  fi
  record PASS "$label" "$(echo "$stderr" | head -1 | cut -c1-80)"
}
