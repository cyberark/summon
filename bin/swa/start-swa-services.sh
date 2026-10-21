#!/usr/bin/env bash
# start-swa-services.sh — Start the SWA Server and SWA Agent in tmux sessions.
#
# Each service runs in its own tmux window so logs remain accessible. The SWA
# Agent socket is exposed at /tmp/swa-e2e-agent.sock. Set SPIFFE_ENDPOINT_SOCKET
# to that path before invoking summon.
#
# Usage:
#   bin/swa/start-swa-services.sh [--detach]
#
# Options:
#   --detach  Start services without attaching tmux (default: attach to agent session)
#
# Environment:
#   SWA_WORKDIR    Working directory containing bootstrap configs (default: /tmp/swa-e2e)
#   SWA_BIN_DIR    Directory containing swa-server and swa-agent binaries (default: /tmp/swa-bins)
#
# Outputs (printed and written to ${SWA_WORKDIR}/env.sh):
#   SPIFFE_ENDPOINT_SOCKET=unix:///tmp/swa-e2e-agent.sock

set -euo pipefail


main() {
  setup_env "$@"
  validate_prerequisites
  kill_existing_sessions
  start_swa_server
  start_swa_agent
  wait_for_agent_socket
  write_env_file
  attach_if_needed
}

announce() { echo ""; echo "=== ${*} ==="; echo ""; }

setup_env() {
  SWA_WORKDIR="${SWA_WORKDIR:-/tmp/swa-e2e}"
  SWA_BIN_DIR="${SWA_BIN_DIR:-/tmp/swa-bins}"
  DETACH=false

  while [[ $# -gt 0 ]]; do
    case $1 in
      --detach) DETACH=true; shift ;;
      *) echo "Unknown flag: $1"; exit 1 ;;
    esac
  done

  BOOTSTRAP_CFG="${SWA_WORKDIR}/bootstrapConfig.yaml"
  AGENT_CFG="${SWA_WORKDIR}/agent/config.yaml"
  JWT_FILE="${SWA_WORKDIR}/server.jwt"

  SWA_SERVER="${SWA_BIN_DIR}/swa-server"
  SWA_AGENT="${SWA_BIN_DIR}/swa-agent"

  SOCKET_PATH="/tmp/swa-e2e-agent.sock"
  ENV_FILE="${SWA_WORKDIR}/env.sh"
}

validate_prerequisites() {
  # ── Validate prerequisites ─────────────────────────────────────────────────

  errors=0
  for f in "$BOOTSTRAP_CFG" "$AGENT_CFG" "$JWT_FILE"; do
    if [[ ! -f "$f" ]]; then
      echo "  MISSING: ${f}" >&2
      errors=$((errors + 1))
    fi
  done
  for b in "$SWA_SERVER" "$SWA_AGENT"; do
    if [[ ! -x "$b" ]]; then
      echo "  NOT FOUND or not executable: ${b}" >&2
      errors=$((errors + 1))
    fi
  done
  if (( errors > 0 )); then
    echo "ERROR: ${errors} prerequisite(s) missing. Run bin/swa/setup-swa.sh first." >&2
    exit 1
  fi

  # Validate JWT is not expired.
  _jwt_b2=$(cut -d. -f2 "${JWT_FILE}" | tr '_-' '/+')  # BSD tr: put - last to avoid option parsing
  JWT_EXP=$(printf '%s==' "${_jwt_b2}" | base64 -d 2>/dev/null | jq -r '.exp // 0' 2>/dev/null || echo "0")
  unset _jwt_b2
  if (( $(date +%s) >= JWT_EXP )); then
    announce "JWT expired — refreshing"
    "$(dirname "$0")/refresh-swa-jwt.sh"
  fi
}

kill_existing_sessions() {
  # ── Kill any existing sessions ─────────────────────────────────────────────

  for session in swa-server swa-agent; do
    if tmux has-session -t "$session" 2>/dev/null; then
      echo "  Killing existing tmux session: ${session}"
      tmux kill-session -t "$session"
    fi
  done

  if [[ -S "$SOCKET_PATH" ]]; then
    echo "  Removing stale agent socket: ${SOCKET_PATH}"
    rm -f "$SOCKET_PATH"
  fi
}

start_swa_server() {
  # ── Start SWA Server ───────────────────────────────────────────────────────

  announce "Starting SWA Server"
  tmux new-session -d -s swa-server -x 220 -y 50
  tmux send-keys -t swa-server \
    "${SWA_SERVER} run --configDir ${SWA_WORKDIR} 2>&1 | tee ${SWA_WORKDIR}/swa-server.log" \
    Enter

  echo "  Server log: ${SWA_WORKDIR}/swa-server.log"
  echo "  tmux session: swa-server"

  # Wait for Server to authenticate to Conjur before starting the Agent.
  # The Server must complete at least one successful auth and start the SPIRE
  # listener (port 8081) before the Agent can attest.
  echo "  Waiting for Server to authenticate (up to 60s)..."
  _server_ready=false
  _server_deadline=$(($(date +%s) + 60))
  while (( $(date +%s) < _server_deadline )); do
    # Success indicators: bound SPIRE listener or broadcasted a bundle
    if grep -q "Broadcasting\|spiffe\|SVID\|api-server" "${SWA_WORKDIR}/swa-server.log" 2>/dev/null; then
      _server_ready=true
      break
    fi
    # Fail fast on persistent 429 (after the initial burst of startup retries)
    _consec=$(grep -c "429" "${SWA_WORKDIR}/swa-server.log" 2>/dev/null) || _consec=0
    if (( _consec > 5 )); then
      echo "  WARNING: Server is hitting rate limits (429 ×${_consec}); Agent start deferred."
      break
    fi
    sleep 2
  done
  if [[ "$_server_ready" == "true" ]]; then
    echo "  Server ready."
  else
    echo "  Server did not reach ready state — proceeding anyway (may be recovering from 429)."
  fi
}

start_swa_agent() {
  # ── Start SWA Agent ────────────────────────────────────────────────────────

  announce "Starting SWA Agent"
  tmux new-session -d -s swa-agent -x 220 -y 50
  tmux send-keys -t swa-agent \
    "${SWA_AGENT} run --configDir ${SWA_WORKDIR}/agent 2>&1 | tee ${SWA_WORKDIR}/swa-agent.log" \
    Enter

  echo "  Agent log: ${SWA_WORKDIR}/swa-agent.log"
  echo "  tmux session: swa-agent"
}

wait_for_agent_socket() {
  # ── Wait for Agent socket ──────────────────────────────────────────────────

  echo ""
  echo "  Waiting for Agent socket at ${SOCKET_PATH} (up to 60s)..."
  deadline=$(($(date +%s) + 60))
  while (( $(date +%s) < deadline )); do
    [[ -S "$SOCKET_PATH" ]] && break
    sleep 1
  done

  if [[ ! -S "$SOCKET_PATH" ]]; then
    echo ""
    echo "ERROR: Agent socket did not appear within 60s." >&2
    echo "  Check logs: ${SWA_WORKDIR}/swa-server.log and ${SWA_WORKDIR}/swa-agent.log" >&2
    exit 1
  fi
  echo "  Socket ready."

  # Wait until the agent has attested and can serve SVIDs (socket ≠ SVIDs ready).
  echo "  Waiting for Agent to issue SVIDs (up to 60s)..."
  SVID_DIR=$(mktemp -d)
  deadline=$(($(date +%s) + 60))
  while (( $(date +%s) < deadline )); do
    if "${SWA_BIN_DIR}/swa-agent" api fetch x509 \
        -s "${SOCKET_PATH}" \
        --write "${SVID_DIR}" --silent 2>/dev/null; then
      echo "  SVIDs ready."
      rm -rf "${SVID_DIR}"
      return
    fi
    sleep 2
  done
  rm -rf "${SVID_DIR}"
  echo "  WARNING: Agent SVID fetch timed out — proceeding; enterprise step may retry." >&2
}

write_env_file() {
  # ── Write env helper ───────────────────────────────────────────────────────

  cat > "$ENV_FILE" <<EOF
# Source this file to export SWA environment for summon.
# Generated by bin/swa/start-swa-services.sh on $(date -u '+%Y-%m-%dT%H:%M:%SZ')
export SPIFFE_ENDPOINT_SOCKET=unix://${SOCKET_PATH}
EOF
  echo ""
  echo "=== Services started ==="
  echo ""
  echo "  Source the env helper before running scenarios:"
  echo "    source ${ENV_FILE}"
  echo ""
  echo "  SPIFFE_ENDPOINT_SOCKET=unix://${SOCKET_PATH}"
  echo ""
}

attach_if_needed() {
  if [[ "$DETACH" == "false" ]]; then
    echo "  Attaching to swa-agent session. Detach with Ctrl-B D."
    sleep 1
    tmux attach-session -t swa-agent
  fi
}

main "$@"
