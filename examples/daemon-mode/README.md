# Running Summon as a daemon (`--refresh`)

`summon --refresh` (aliases: `--daemon`, `--sidecar`) runs Summon as a
long-lived process: it launches your subprocess once, then keeps polling the
provider on an interval. When a secret changes, Summon rewrites any
push-to-file secrets and, by default, gracefully restarts the subprocess so it
starts with the new values. This is the pattern for a service or sidecar that
should pick up rotated credentials without a manual restart.

Daemon mode is not supported on Solaris; `--refresh` exits with an error there.

Daemon mode is configured through environment variables:

| Variable                     | Purpose                                                                                     | Default |
|------------------------------|---------------------------------------------------------------------------------------------|---------|
| `SUMMON_REFRESH_INTERVAL`    | How often to poll the provider (minimum `5s`)                                                | `5m`    |
| `SUMMON_RESTART_ON_ROTATION` | Restart the subprocess when a secret changes; `false` = poll and rewrite files only          | `true`  |
| `SUMMON_REFRESH_TIMEOUT`     | Max time a single poll may take; exceeding it kills the provider (or aborts the Conjur request) and counts as a failure | `60s`   |
| `SUMMON_TERM_GRACE_PERIOD`   | Time a restart or shutdown waits for the subprocess tree to stop before force-killing it                 | `30s`   |
| `SUMMON_MAX_REFRESH_OUTAGE`  | Wall-clock duration of continuous failed polls before Summon stops the subprocess and exits with code `2`; `0` = unlimited, otherwise at least the refresh interval | `0` |
| `SUMMON_MAX_AUTH_FAILURES`   | Consecutive polls answered with HTTP 401 (e.g. after the host API key was rotated) before Summon stops the subprocess and exits with code `3`; default `5`, `0` disables |
| `SUMMON_LOG_LEVEL`           | `DEBUG`, `INFO`, `WARN` or `ERROR`                                                            | `INFO`  |

Secret values never appear in logs at any level.

## Try it

The files in this directory are a self-contained demo that needs no Conjur:

| File | Role |
|---|---|
| `secrets.yml` | `DB_PASSWORD` (env-injected) and `TLS_CERT` (push-to-file, written to `tls/cert.pem`) |
| `secrets-files-only.yml` | Same `TLS_CERT` file secret, no env secrets |
| `provider` | Demo provider: the secret `db-password` is the content of `values/db-password` |
| `app.sh` | Demo workload: prints the secrets it started with, then idles until stopped |

Build Summon, then from this directory:

```sh
go build -o summon ../../cmd  # from this directory
export SUMMON_REFRESH_INTERVAL=5s
./summon --refresh -p ./provider ./app.sh
```

Summon checks the provider before every poll: `provider` and every directory above
it must be owned by you (or root) and not group- or world-writable, otherwise the poll
fails. A checkout in a shared or group-writable directory will not work; run the demo
from a directory only you can write to. On macOS and other non-Linux systems Summon
also prints one startup line, `summon: warning: --refresh: process is not marked
non-dumpable ...` (Windows prints a similar line listing the checks it lacks; Linux
prints none).

In a second terminal simulate a rotation by changing a value:

```sh
echo rotated-password > values/db-password
```

Within one interval Summon stops `app.sh` (`SIGTERM`, then `SIGKILL` after the
grace period) and starts it again with the new value:

```
app started (pid 65655): DB_PASSWORD=initial-password cert=initial-cert
level=INFO msg="Secrets changed" changed_variables=[DB_PASSWORD] updated_files=[]
app stopping (pid 65655)
app started (pid 65687): DB_PASSWORD=rotated-password cert=initial-cert
```

`echo rotated-cert > values/tls-cert` rewrites `tls/cert.pem` first and then
restarts the app the same way. The file secret is a
[push-to-file](../../README.md#push-to-file) secret: it is rewritten only when
the rendered content's checksum changes.

## File-injected secrets: rewrite only

If your application re-reads its secret files itself, disable the restart:

```sh
export SUMMON_RESTART_ON_ROTATION=false
./summon --refresh -f secrets-files-only.yml -p ./provider ./app.sh
echo rotated-cert > values/tls-cert   # tls/cert.pem changes; app.sh is not restarted
```

Env-injected secrets are *not* refreshed in this mode, since a running
process's environment cannot change.

Restore the demo values with `echo initial-password > values/db-password` and
`echo initial-cert > values/tls-cert`.

## Windows

Restart works on Windows with these limits:

- The graceful stop (`CTRL_BREAK`) reaches console processes only. Services,
  GUI apps, or anything ignoring `CTRL_BREAK` is force-killed when the grace
  period expires.
- The child runs in its own process group, so it ignores Ctrl+C; the workload
  must handle `CTRL_BREAK`.
- If Summon has no console (for example when run as a Windows service), it
  skips the graceful stop and force-kills the tree on every restart. Summon
  logs a warning at startup.

## Running the acceptance tests

```sh
go build -o /tmp/summon-bin/summon ./cmd
cd acceptance && PATH=/tmp/summon-bin:$PATH cucumber features/daemon_mode.feature
```
