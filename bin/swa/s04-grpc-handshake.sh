#!/usr/bin/env bash
# s04-grpc-handshake.sh — Scenario 4: Socket exists; gRPC handshake fails
#
# Reproduces a gRPC framing error by binding a Unix socket that accepts TCP
# connections but immediately closes them. The gRPC client receives invalid
# framing data and returns a connection error — distinct from the ENOENT in
# scenario 3. Also verifies that the test secret does not appear in the error
# output.
#
# Note: this scenario always runs regardless of SWA_EDGE_URL — it uses a local
# fake socket server, not real Edge infrastructure.

# ┌──────────────────────────────────────────────────────────────────────────┐
# │ Scenario 4 — Socket exists; gRPC handshake fails                         │
# │                                                                          │
# │ Reproduce by binding a Unix domain socket that accepts TCP connections   │
# │ but does not speak gRPC. The gRPC client receives invalid framing data   │
# │ and returns a connection error — distinct from the ENOENT in scenario 3. │
# └──────────────────────────────────────────────────────────────────────────┘

echo "Running scenario 4: socket exists, gRPC handshake fails..."
FAKE_SOCK="/tmp/swa-e2e-fake-grpc.sock"
rm -f "$FAKE_SOCK"

# Start a Unix socket listener that accepts connections and immediately closes them,
# causing gRPC to report a framing or EOF error rather than ENOENT.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 "${SCRIPT_DIR}/fake-unix-socket-server.py" "$FAKE_SOCK" &
FAKE_PID=$!

# Wait for the socket file to appear.
deadline=$(($(date +%s) + 5))
while (( $(date +%s) < deadline )) && [[ ! -S "$FAKE_SOCK" ]]; do sleep 0.1; done

if [[ -S "$FAKE_SOCK" ]]; then
  # Expected error contains a gRPC framing indicator, not ENOENT — confirms the socket
  # was reached but gRPC handshake failed, distinct from scenario 3.
  run_summon_sad \
    "S4: socket exists; gRPC handshake fails" \
    "EOF\|connection reset\|transport\|grpc\|workload" \
    "CONJUR_APPLIANCE_URL=${CONJUR_APPLIANCE_URL}" \
    "CONJUR_ACCOUNT=${CONJUR_ACCOUNT}" \
    "CONJUR_AUTHN_JWT_SERVICE_ID=${CONJUR_AUTHN_JWT_SERVICE_ID}" \
    "SPIFFE_ENDPOINT_SOCKET=unix://${FAKE_SOCK}" \
    "CONJUR_AUTHN_TOKEN="
else
  record FAIL "S4: socket exists; gRPC handshake fails" "could not create fake socket within 5s"
fi

kill "$FAKE_PID" 2>/dev/null; wait "$FAKE_PID" 2>/dev/null || true
rm -f "$FAKE_SOCK"
unset FAKE_SOCK FAKE_PID deadline
