# SWA E2E Test Runbook

Manual execution guide for the Summon + SWA (Secure Workload Access) E2E test harness.
A developer with access to a Conjur Cloud tenant can follow this guide from scratch
to run the SWA E2E scenario suite.

---

## Prerequisites

### Required tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `conjur` CLI | Log in to Conjur Cloud | Available from the Conjur Cloud downloads page |
| `openssl` | PKI generation | Install via Homebrew (`brew install openssl`) or package manager |
| `jq` | JSON processing in scripts | `brew install jq` or package manager |
| `python3` | RSA key conversion (`build-jwks.py`) | Python 3.8 or later; no third-party packages required |
| `tmux` | Running SWA Server and Agent in background sessions | `brew install tmux` |
| `curl` | Conjur REST API calls | Pre-installed on most systems |
| `docker` / `docker compose` | SWA Edge local instance (optional — Edge scenarios only) | Docker Desktop or Colima |
| `summon` | The binary under test | Build from source (see below) |

### Required access

- A Conjur Cloud tenant with SWA enabled and admin credentials (OIDC login).
- The `swa-server` and `swa-agent` binaries for your platform, placed in `SWA_BIN_DIR`
  (default: `/tmp/swa-bins`).

### Build summon from source

```sh
cd /path/to/summon
go build -o /usr/local/bin/summon ./cmd/summon
summon --version
```

---

## Environment variable reference

### Required for all scenarios

| Variable | Example | Description |
|----------|---------|-------------|
| `CONJUR_APPLIANCE_URL` | `https://mytenant.secretsmgr.cyberark.cloud` | Bare tenant URL — no `/api` suffix, no trailing slash |
| `CONJUR_ACCOUNT` | `conjur` | Conjur account name |
| `CONJUR_AUTHN_JWT_SERVICE_ID` | `swa-e2e-jwt` | authn-jwt service ID provisioned by `bin/swa/setup-swa.sh` |
| `SPIFFE_ENDPOINT_SOCKET` | `unix:///tmp/swa-e2e-agent.sock` | SWA Agent workload API socket |

### Optional — Edge scenarios

| Variable | Default | Description |
|----------|---------|-------------|
| `SWA_EDGE_URL` | _(empty)_ | SWA Edge proxy URL; Edge scenarios are skipped when empty |
| `SWA_EDGE_CA_CERT` | _(empty)_ | PEM-encoded CA cert for the Edge TLS connection |

### Optional — Enterprise (self-hosted Conjur) scenarios

| Variable | Default | Description |
|----------|---------|-------------|
| `CONJUR_ENTERPRISE_URL` | _(empty)_ | Self-hosted Conjur appliance URL; Enterprise scenarios are skipped when empty |
| `CONJUR_ENTERPRISE_ACCOUNT` | `conjur` | Account on the self-hosted appliance |
| `CONJUR_ENTERPRISE_JWT_SERVICE_ID` | _(same as JWT service ID)_ | authn-jwt service ID on the self-hosted appliance |
| `CONJUR_ENTERPRISE_CERT_SERVICE_ID` | _(same as cert service ID)_ | authn-x509 service ID on the self-hosted appliance |
| `CONJUR_ENTERPRISE_CA_CERT` | _(empty)_ | PEM CA cert for the appliance TLS connection |
| `CONJUR_ENTERPRISE_JWT_HOST_ID` | _(empty)_ | Host ID override for JWT auth on the appliance |
| `CONJUR_ENTERPRISE_CERT_HOST_ID` | _(empty)_ | Host ID override for cert auth on the appliance |
| `CONJUR_ENTERPRISE_IGNORE_SSL` | `false` | Set to `true` to skip TLS verification (CI only, not for production) |
| `CONJUR_ENTERPRISE_GODEBUG` | _(empty)_ | Go `GODEBUG` flags forwarded to summon for Enterprise scenarios. Set to `x509negativeserial=1` when running against Conjur 5.x, which generates certificates with DER-negative serial numbers that Go 1.20+ rejects by default. |

> **`CONJUR_ADMIN_PASSWORD` is required** for `bin/swa/setup-swa-enterprise.sh` — there is no default. Set it before running:
> ```sh
> export CONJUR_ADMIN_PASSWORD=<your-local-appliance-password>
> ```

### Script defaults

| Variable | Default | Description |
|----------|---------|-------------|
| `CONJURRC_FILE` | `~/.conjurrc-swa` | Conjur CLI conjurrc for the SWA tenant |
| `NETRC_FILE` | `~/.netrc-swa` | Conjur CLI netrc for the SWA tenant |
| `SWA_WORKDIR` | `/tmp/swa-e2e` | Working directory for PKI, configs, and logs |
| `SWA_BIN_DIR` | `/tmp/swa-bins` | Directory containing `swa-server` and `swa-agent` binaries |
| `TRUST_DOMAIN` | `swa-e2e.conjur.cloud` | SPIFFE trust domain |
| `JWT_SERVICE_ID` | `swa-e2e-jwt` | authn-jwt service ID (matches `CONJUR_AUTHN_JWT_SERVICE_ID`) |
| `CERT_SERVICE_ID` | `swa-e2e-cert` | authn-x509 service ID (matches `CONJUR_AUTHN_CERT_SERVICE_ID`) |
| `TEST_VAR_PATH` | `data/swa-e2e/test-secret` | Conjur variable path used by scenario scripts |
| `SUMMON_BINARY` | `summon` | Override to use a non-PATH summon binary |

---

## IMPORTANT: the `controlPlane.url` gotcha

`bootstrapConfig.yaml` must contain the **bare tenant URL with no `/api` suffix**.

```yaml
# CORRECT
controlPlane:
  url: https://mytenant.secretsmgr.cyberark.cloud

# WRONG — causes 404 on every SWA Server auth call
controlPlane:
  url: https://mytenant.secretsmgr.cyberark.cloud/api
```

`bin/swa/setup-swa.sh` and `bin/swa/start-swa-services.sh` both strip the `/api` suffix automatically.
If you edit `bootstrapConfig.yaml` manually, verify the URL before starting SWA Server.

---

## Setup sequence

### Step 1 — Log in to Conjur Cloud

```sh
CONJURRC=~/.conjurrc-swa conjur login
# Enter your OIDC credentials when prompted.
# The CLI writes the session token to ~/.netrc-swa.
```

Verify:

```sh
CONJURRC=~/.conjurrc-swa conjur whoami
```

### Step 2 — Place SWA binaries

Place the `swa-server` and `swa-agent` binaries in `SWA_BIN_DIR`:

```sh
SWA_BIN_DIR=/tmp/swa-bins
mkdir -p "$SWA_BIN_DIR"
# Copy or move swa-server and swa-agent to $SWA_BIN_DIR, then:
chmod +x "$SWA_BIN_DIR/swa-server" "$SWA_BIN_DIR/swa-agent"
```

### Step 3 — Provision SWA resources and configure Conjur

```sh
export CONJUR_APPLIANCE_URL=https://mytenant.secretsmgr.cyberark.cloud
export CONJUR_ACCOUNT=conjur
export SWA_BIN_DIR=/tmp/swa-bins

bin/swa/setup-swa.sh
```

This script:
- Generates PKI (RSA-2048 JWT key pair + EC P-256 x509pop CA and agent cert).
- Provisions the SWA trust domain, server group, server, and node group on Conjur Cloud.
- Loads the `authn-jwt/swa-e2e-jwt` and `authn-x509/swa-e2e-cert` Conjur policies.
- Sets all authenticator variables (inline JWKS, issuer, CA cert, etc.).
- Creates and seeds the test secret `data/swa-e2e/test-secret`.
- Writes `bootstrapConfig.yaml` and `agent/config.yaml` to `SWA_WORKDIR`.

Runtime: approximately 1 to 2 minutes.

### Step 4 — Start SWA Server and Agent

```sh
bin/swa/start-swa-services.sh --detach
```

- SWA Server runs in a tmux session named `swa-server`.
- SWA Agent runs in a tmux session named `swa-agent`.
- Logs are written to `${SWA_WORKDIR}/swa-server.log` and `swa-agent.log`.
- The SWA Agent socket appears at `/tmp/swa-e2e-agent.sock` within 20 seconds.

Verify the socket is present:

```sh
test -S /tmp/swa-e2e-agent.sock && echo "Socket ready."
```

To inspect logs:

```sh
tmux attach-session -t swa-agent    # detach: Ctrl-B D
tmux attach-session -t swa-server   # detach: Ctrl-B D
```

---

## Running scenarios

Export required variables:

```sh
export CONJUR_APPLIANCE_URL=https://mytenant.secretsmgr.cyberark.cloud
export CONJUR_ACCOUNT=conjur
export CONJUR_AUTHN_JWT_SERVICE_ID=swa-e2e-jwt
export SPIFFE_ENDPOINT_SOCKET=unix:///tmp/swa-e2e-agent.sock
export SWA_WORKDIR=/tmp/swa-e2e
```

Run all scenarios:

```sh
bin/swa/run-swa-scenarios.sh
```

The script prints a pass/fail table and exits non-zero if any scenario fails.
Copy the output and attach it as the test execution record.

---

## Scenario notes

| # | Scenario | Requires | Notes |
|---|----------|----------|-------|
| 1 | JWT-SVID → authn-jwt → secret retrieved | SWA services running | Calls `FetchJWTSVID` via SPIFFE Workload API; also verifies `SUMMON_PROVIDER=conjur` env routing |
| 2 | cert auth via SWA Edge → secret retrieved | SWA Edge URL set in `SWA_EDGE_URL` | Skipped when `SWA_EDGE_URL` is empty |
| 3 | missing socket → error with socket path in message | — | Verifies no secret in output |
| 4 | socket present, wrong protocol → gRPC handshake error | `fake-unix-socket-server.py` | Verifies no secret in output |
| 5 | `CONJUR_AUTHN_JWT_SERVICE_ID` absent → config error | — | Verifies error names the missing variable; no secret in output |
| 6 | `CONJUR_APPLIANCE_URL` unset → config error | — | Verifies error names missing variable |
| 7 | cert files missing → config error | — | Verifies error names missing path |
| 8 | JWT-SVID → self-hosted Conjur enterprise → secret retrieved | `CONJUR_ENTERPRISE_URL` set, enterprise appliance running | Skipped when `CONJUR_ENTERPRISE_URL` is empty |
| 9 | cert auth → self-hosted Conjur enterprise → secret retrieved | `CONJUR_ENTERPRISE_URL` set, enterprise appliance running | Skipped when `CONJUR_ENTERPRISE_URL` is empty |

---

## Refreshing the SWA Server JWT

The SWA Server JWT has a 1-hour lifetime. Refresh it at any time:

```sh
bin/swa/refresh-swa-jwt.sh
```

`bin/swa/start-swa-services.sh` calls this automatically on startup.

---

## Restarting services

Stop all SWA sessions:

```sh
tmux kill-session -t swa-server
tmux kill-session -t swa-agent
```

Restart:

```sh
bin/swa/start-swa-services.sh --detach
```

---

## Troubleshooting

### SWA Server fails to start

1. Check `${SWA_WORKDIR}/swa-server.log` for error messages.
2. Verify `controlPlane.url` in `bootstrapConfig.yaml` has **no** `/api` suffix.
3. Verify the Server JWT is not expired: `bin/swa/refresh-swa-jwt.sh`.
4. Confirm the Conjur Cloud tenant is reachable from the test machine.

### SWA Agent socket does not appear

1. Check `${SWA_WORKDIR}/swa-agent.log`.
2. Confirm SWA Server is running: `tmux list-sessions | grep swa-server`.
3. Check `agent/config.yaml` — `serverAddress` must match the Server listen address.

### S2 or S9 — `authn-cert` returns 401

The Firefly CA rotates every time the SWA Server restarts. Edge must replicate the new CA
before cert auth works.

1. Re-run `bin/swa/setup-swa.sh` (Step 13 updates the `authn-cert/<id>/ca-cert` variable
   in Conjur Cloud from the live Agent bundle). The same command also re-runs all other
   setup steps safely.
2. Wait ~60 s for Edge to replicate the new CA, then re-run the scenarios.

If the error is `No valid credentials found` rather than `401 Unauthorized`, a caller-shell
credential variable (`NETRC_FILE` or `CONJUR_NETRC_PATH`) is leaking into the summon
subprocess and interfering with the SPIFFE cert auth path. The scenario scripts already
clear these; verify that no outer script re-exports them after sourcing `enterprise-env.sh`.

### `authn-jwt` returns 401

1. Confirm the JWT service ID matches `CONJUR_AUTHN_JWT_SERVICE_ID`.
2. Confirm the host annotation `authn-jwt/<service-id>/sub` matches the SPIFFE ID
   returned by `FetchJWTSVID`. Inspect logs or add debug logging with `SUMMON_DEBUG=true`.
3. Confirm the JWKS public-keys variable in Conjur matches the key pair in `SWA_WORKDIR`.
   Re-run `bin/swa/setup-swa.sh` if the PKI was regenerated.

---

## Cleaning up

Remove working directory:

```sh
tmux kill-session -t swa-server 2>/dev/null || true
tmux kill-session -t swa-agent  2>/dev/null || true
rm -rf /tmp/swa-e2e /tmp/swa-e2e-agent.sock
```

Conjur Cloud resources (trust domain, policies) persist until deleted manually via the UI
or REST API.
