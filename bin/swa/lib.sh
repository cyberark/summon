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
