# summon

<div align="center">
  <a href="https://cyberark.github.io/summon">
    <img src="https://cyberark.github.io/summon/images/logo.png" height="200"/><br>
    cyberark.github.io/summon
  </a>
</div>

> **Note:** The cyberark.github.io/summon documentation site is no longer maintained.
> This README is the authoritative source for configuration and usage information.

[![GitHub release](https://img.shields.io/github/release/cyberark/summon.svg)](https://github.com/cyberark/summon/releases/latest)

[![Github commits (since latest release)](https://img.shields.io/github/commits-since/cyberark/summon/latest.svg)](https://github.com/cyberark/summon/commits/main)

---

`summon` is a command-line tool to make working with secrets easier.

It provides an interface for

* Reading a `secrets.yml` file
* Fetching secrets from a trusted store
* Exporting secret values to a sub-process environment
* Writing secrets to files in various formats (push-to-file)

## Install

Note installing `summon` alone is not sufficient; you need to also install
a [provider of your choice](#providers) before it's ready for use.

Pre-built binaries and packages are available from GitHub releases
[here](https://github.com/cyberark/summon/releases).

### Using Summon with CyberArk Secrets Manager

Summon ships with a **built-in Conjur provider**. No separate binary install is needed; it activates automatically when the provider directory has no other binaries, or explicitly when you pass `--provider conjur`.

> **Note:** If the provider directory contains exactly one binary, Summon uses that binary automatically. If it contains two or more, Summon exits with an error and prompts you to specify one with `--provider`. The built-in Conjur provider is only used as the automatic fallback when the directory is empty.

Are you using this project with [Conjur Open Source](https://github.com/cyberark/conjur)? Then we
**strongly** recommend choosing the version of this project to use from the latest [Conjur OSS
suite release](https://docs.conjur.org/Latest/en/Content/Overview/Conjur-OSS-Suite-Overview.html).
Conjur maintainers perform additional testing on the suite release versions to ensure
compatibility. When possible, upgrade your Conjur version to match the
[latest suite release](https://docs.conjur.org/Latest/en/Content/ReleaseNotes/ConjurOSS-suite-RN.htm);
when using integrations, choose the latest suite release that matches your Conjur version. For any
questions, please contact us on [Discourse](https://discuss.cyberarkcommons.org/c/conjur/5).

#### Quick Start

Define a `secrets.yml` file:

```yaml
DB_PASSWORD: !var production/db/password
API_KEY: !var production/api/key
```

Set the required environment variables and run your command:

```sh
export CONJUR_APPLIANCE_URL=https://conjur.example.com
export CONJUR_ACCOUNT=myorg
export CONJUR_AUTHN_LOGIN=host/myapp
export CONJUR_AUTHN_API_KEY=<api-key>

summon -f secrets.yml env | grep -E 'DB_PASSWORD|API_KEY'
```

No `--provider` flag is needed; Summon detects the built-in Conjur provider automatically.

For other authentication methods, see [Supported Authentication Methods](#supported-authentication-methods) below.

#### Supported Authentication Methods

The built-in provider supports the following authentication methods via `CONJUR_AUTHN_TYPE`:

| Auth method | `CONJUR_AUTHN_TYPE` | Required additional env vars |
|---|---|---|
| API key (default) | *(unset)* | `CONJUR_AUTHN_LOGIN`, `CONJUR_AUTHN_API_KEY` |
| Existing token | *(unset)* | `CONJUR_AUTHN_TOKEN` or `CONJUR_AUTHN_TOKEN_FILE` |
| JWT | `jwt` | `CONJUR_AUTHN_JWT_SERVICE_ID`, `JWT_TOKEN_PATH` or `CONJUR_AUTHN_JWT_TOKEN`, `CONJUR_AUTHN_JWT_HOST_ID` (required if token-app-property not configured) |
| AWS IAM | `iam` | `CONJUR_SERVICE_ID`, `CONJUR_AUTHN_JWT_HOST_ID` |
| Azure | `azure` | `CONJUR_SERVICE_ID`, `CONJUR_AUTHN_JWT_HOST_ID`, `CONJUR_AUTHN_AZURE_CLIENT_ID` (optional) |
| GCP | `gcp` | `CONJUR_AUTHN_JWT_HOST_ID` (required when auto-fetching token from GCP metadata service) |
| Certificate (authn-cert) | `cert` (implied when `CONJUR_AUTHN_CERT_SERVICE_ID` is set) | `CONJUR_AUTHN_CERT_SERVICE_ID`, `CONJUR_AUTHN_CERT_FILE`, `CONJUR_AUTHN_CERT_KEY_FILE`, `CONJUR_AUTHN_CERT_HOST_ID` (optional) |

#### Configuration

The provider loads configuration from:

- `.conjurrc` in the home directory, current directory, or the path set in `CONJUR_RC`
- `/etc/conjur.conf` on Linux/macOS; `C:\Windows\conjur.conf` on Windows
- Environment variables (take precedence over file-based config)

##### Connection

| Variable | Description |
|---|---|
| `CONJUR_APPLIANCE_URL` | URL of the Conjur appliance |
| `CONJUR_ACCOUNT` | Conjur organization account |
| `CONJUR_CERT_FILE` | Path to the Conjur SSL certificate file |
| `CONJUR_SSL_CERTIFICATE` | PEM-encoded SSL certificate (inline) |

> **SSL certificate precedence:** when both `CONJUR_SSL_CERTIFICATE` and `CONJUR_CERT_FILE` are set,
> `CONJUR_SSL_CERTIFICATE` (inline PEM) takes precedence over `CONJUR_CERT_FILE`. Only set one to
> avoid confusion.

> **`CONJUR_CERT_FILE` vs `CONJUR_AUTHN_CERT_FILE`:** `CONJUR_CERT_FILE` is the Conjur server's
> TLS certificate used to verify the HTTPS connection. `CONJUR_AUTHN_CERT_FILE` is the client
> certificate presented to Conjur during certificate authentication (mTLS). They serve different
> purposes and can both be set at the same time.

##### Authentication

| Variable | Description |
|---|---|
| `CONJUR_AUTHN_LOGIN` | Conjur identity (user or host) |
| `CONJUR_AUTHN_API_KEY` | API key for the identity |
| `CONJUR_AUTHN_TOKEN` | Pre-fetched Conjur auth token (inline) |
| `CONJUR_AUTHN_TOKEN_FILE` | Path to a file containing the auth token. At startup summon waits up to `SUMMON_AUTHN_TOKEN_FILE_WAIT` for a missing file to appear, then exits with a configuration error; a path that is a directory or unreadable fails immediately. Once running, the library re-reads the file when it changes; if it disappears or becomes unreadable later, the fetch fails with an error naming `CONJUR_AUTHN_TOKEN_FILE` and the path, and succeeds again once the file is back; in `--refresh` mode that is an ordinary failed poll |
| `SUMMON_AUTHN_TOKEN_FILE_WAIT` | Go duration (default `30s`, `0` = do not wait) that summon waits at startup for `CONJUR_AUTHN_TOKEN_FILE` to exist; negative or malformed values are a configuration error |
| `CONJUR_AUTHN_JWT_SERVICE_ID` | JWT authenticator service ID (e.g. `kubernetes`) |
| `JWT_TOKEN_PATH` | Path to the JWT token file (default: `/var/run/secrets/kubernetes.io/serviceaccount/token`) |
| `CONJUR_AUTHN_TYPE` | Auth backend: `jwt`, `iam`, `azure`, `gcp`, or `cert`. Setting `CONJUR_AUTHN_CERT_SERVICE_ID` activates cert authentication implicitly, overriding any other value set here. |
| `CONJUR_SERVICE_ID` | Authenticator service ID for IAM and Azure. Also accepted for `jwt` and `cert`, but prefer the dedicated `CONJUR_AUTHN_JWT_SERVICE_ID` / `CONJUR_AUTHN_CERT_SERVICE_ID`, which override it. |
| `CONJUR_AUTHN_JWT_HOST_ID` | Conjur host identity for IAM, Azure, and JWT auth; for GCP, required only when auto-fetching the token from the GCP metadata service |
| `CONJUR_AUTHN_AZURE_CLIENT_ID` | Azure managed identity client ID (optional; selects a specific identity on multi-identity hosts) |
| `CONJUR_AUTHN_JWT_TOKEN` | Inline JWT token string (alternative to `JWT_TOKEN_PATH`; use when the token is available as a value rather than a file) |
| `CONJUR_AUTHN_CERT_SERVICE_ID` | Certificate authenticator service ID. Setting this implicitly activates cert authentication (equivalent to `CONJUR_AUTHN_TYPE=cert`) and takes precedence over `CONJUR_SERVICE_ID`. |
| `CONJUR_AUTHN_CERT_FILE` | Path to the PEM-encoded client certificate file for certificate authentication (mTLS). |
| `CONJUR_AUTHN_CERT_KEY_FILE` | Path to the PEM-encoded private key file for the client certificate. |
| `CONJUR_AUTHN_CERT_HOST_ID` | Conjur host identity for certificate authentication (e.g. `host/vm-workloads/vm-01`). Optional in SPIFFE mode — leave it unset and the server derives the host from the certificate's SAN URI. |

> **Certificate authentication requires an HTTPS appliance URL and is not supported on Conjur Cloud
> (Idira Secrets Manager, SaaS).** Both are rejected at config validation, before any request is
> sent.

##### SPIFFE Workload API (zero-static-credential)

Set these variables to authenticate using a SPIFFE/SPIRE platform identity with no API key,
certificate file, or token path. Both X.509-SVID (`authn-cert`) and JWT-SVID (`authn-jwt`)
paths are supported.

| Variable | Description |
|---|---|
| `SPIFFE_ENDPOINT_SOCKET` | Required. Address of the SPIRE agent socket (e.g. `unix:///run/spire/api.sock`). Triggers SPIFFE auto-wiring in `conjur-api-go`. This variable is passed through to the child process. |
| `CONJUR_AUTHN_CERT_SERVICE_ID` | X.509-SVID path. Selects the Conjur `authn-cert` authenticator service ID and implies `AuthnType=cert`. When `SPIFFE_ENDPOINT_SOCKET` is also set, `conjur-api-go` sources the certificate from the Workload API instead of a file. |
| `CONJUR_AUTHN_JWT_SERVICE_ID` | JWT-SVID path. Selects the Conjur `authn-jwt` authenticator service ID. When `SPIFFE_ENDPOINT_SOCKET` is also set and no static JWT token is configured, `conjur-api-go` sources the token from the Workload API automatically. |
| `CONJUR_SPIFFE_ID` | Optional. Selects a specific SVID by SPIFFE ID when the Workload API returns more than one (e.g. `spiffe://example.org/workload/myapp`). Required only when multiple SVIDs are registered for the workload. |

**Which path is used** depends on which service ID is set:
- `CONJUR_AUTHN_CERT_SERVICE_ID` set → X.509-SVID (certificate-based mTLS)
- `CONJUR_AUTHN_JWT_SERVICE_ID` set (and no static token) → JWT-SVID (token-based)

**Conjur-side prerequisites for X.509-SVID:**
1. Enable `authn-cert/<service-id>` on the Conjur server.
2. Configure the webservice with the SPIRE trust bundle CA as the accepted CA.
3. Grant the SPIFFE host identity `read` and `authenticate` on the webservice.

**Conjur-side prerequisites for JWT-SVID:**
1. Enable `authn-jwt/<service-id>` on the Conjur server.
2. Configure `public-keys`, `issuer`, `audience`, and `token-app-property` variables
   under the authenticator policy.
3. Grant the workload host `read` and `authenticate` on the webservice.

See `bin/setup-spiffe.sh` and `ci/spiffe/` for a reference SPIRE configuration used in
the integration test harness.

##### Credential storage

| Variable | Description |
|---|---|
| `CONJUR_CREDENTIAL_STORAGE_MODE` | Read/write policy: `readonly` (built-in provider default) or `readwrite`. Case-insensitive. |
| `CONJUR_CREDENTIAL_STORAGE` | Storage backend: `keyring` (default), `file`, or `none` |
| `CONJUR_NETRC_PATH` | Path to a readable `.netrc` file when using `file` storage. When set, conjur-api-go reads credentials from that path instead of the default location; when unset, the default netrc location is used. |
| `CONJUR_KEYCHAIN_NAMESPACE` | Optional keyring service namespace for concurrent invocations. Non-empty string without `/`, `\`, `:`, or null bytes. When set, the keyring service name becomes `machineName:namespace`; when unset, the existing machine name is used (no namespace isolation). Also configurable as `keychain_namespace` in `.conjurrc`. |

#### Credential storage

The built-in provider uses [conjur-api-go](https://github.com/cyberark/conjur-api-go) credential caching. Two settings work together:

1. **Storage mode** — whether the configured backend may **write** cached credentials.
2. **Storage backend** — where cached credentials are stored (`keyring`, `file`, or `none`).

##### Storage mode (read/write policy)

By default, the built-in provider runs in **read-only** mode: it may read existing cached credentials from the configured backend but does **not** write new ones to the OS keychain or `~/.netrc`. This reduces cross-talk when multiple summon processes run concurrently.

| Mode | Behavior |
|---|---|
| `readonly` (default) | Read cached credentials; suppress writes to keychain/netrc. |
| `readwrite` | Read and write cached credentials (previous default behavior). |

Set the mode with `CONJUR_CREDENTIAL_STORAGE_MODE`:

```sh
export CONJUR_CREDENTIAL_STORAGE_MODE=readwrite
```

##### Storage backends

When mode is `readwrite`, credentials are cached using one of three backends:

| Backend | When to use |
|---|---|
| `keyring` (default) | OS credential stores: macOS Keychain, Windows Credential Manager, Linux Secret Service. For concurrent invocations that share a host keyring, set `CONJUR_KEYCHAIN_NAMESPACE` (or `keychain_namespace` in `.conjurrc`) so each process uses a distinct service name (`machineName:namespace`). Leave unset to keep the existing machine-name service (no namespace isolation). Namespace values must be non-empty and must not contain `/`, `\`, `:`, or null bytes. |
| `file` | Stores credentials in `~/.netrc` by default, or in the readable file path given by `CONJUR_NETRC_PATH` / `netrc_path` when set. Use a per-invocation path to isolate netrc files across processes. Used as fallback when keyring is unavailable, or set explicitly for compatibility. |
| `none` | Disables the storage backend entirely. Use when no credential cache is desired. |

Configure the backend in `.conjurrc`:

```yaml
credential_storage: keyring
keychain_namespace: service-a
```

```yaml
credential_storage: file
netrc_path: /path/to/.netrc
```

Or via environment variables:

```sh
export CONJUR_CREDENTIAL_STORAGE=keyring
export CONJUR_KEYCHAIN_NAMESPACE=service-a

export CONJUR_CREDENTIAL_STORAGE=file
export CONJUR_NETRC_PATH=/path/to/.netrc
```

##### Enable credential writes to disk

To restore write caching (for example, a single long-lived host identity that should reuse credentials across invocations):

```sh
export CONJUR_CREDENTIAL_STORAGE_MODE=readwrite
export CONJUR_CREDENTIAL_STORAGE=file          # optional; keyring is the default backend
export CONJUR_NETRC_PATH="$HOME/.summon-netrc" # optional; defaults to ~/.netrc
```

With `readonly` mode (the default), summon still authenticates using credentials supplied in the environment (`CONJUR_AUTHN_API_KEY`, `CONJUR_AUTHN_TOKEN`, and so on) and may read an existing cache, but it will not update keychain or netrc files.

#### Migration from standalone summon-conjur

If you previously installed `summon-conjur` as a provider binary in your provider directory
and relied on Summon's automatic provider detection (no `--provider` flag), follow these steps:

1. Remove the `summon-conjur` binary from your provider directory
   (typically `/usr/local/lib/summon/`).
2. Summon will use the built-in Conjur provider automatically.

If the binary cannot be removed (for example, it is managed by a system package manager), pass `--provider conjur` explicitly to select the built-in provider regardless of what is in the provider directory.

If you were passing `--provider conjur` explicitly, the flag still works. You can also drop it; the built-in provider is now the default.

### Homebrew

```
brew tap cyberark/tools
brew install summon
```

### Linux (Debian and Red Hat flavors)

`deb` and `rpm` files are attached to new releases.
These can be installed with `dpkg -i summon_v*.deb` and
`rpm -ivh summon_v*.rpm`, respectively.

### Auto Install

**Note** Check the
[release notes](https://github.com/cyberark/summon/releases) and select an
appropriate release to ensure support for your version of CyberArk Secrets Manager.

Use the auto-install script. This will install the latest version of summon.
The script requires sudo to place summon in `/usr/local/bin`.

```
curl -sSL https://raw.githubusercontent.com/cyberark/summon/main/install.sh | bash
```

### Manual Install
Otherwise, download the [latest release](https://github.com/cyberark/summon/releases) and extract it to `/usr/local/bin/summon`.

## Usage

By default, summon will look for `secrets.yml` in the directory it is
called from and export the secret values to the environment of the command it wraps.

*Example*

You want to run a script that requires AWS keys to list your EC2 instances.

Define your keys in a `secrets.yml` file

```yml
AWS_ACCESS_KEY_ID: !var aws/iam/user/robot/access_key_id
AWS_SECRET_ACCESS_KEY: !var aws/iam/user/robot/secret_access_key
```

The script uses the Python library [boto](https://pypi.python.org/pypi/boto), which looks for `AWS_ACCESS_KEY_ID`
and `AWS_SECRET_ACCESS_KEY` in the environment.

```python
import boto
botoEC2 = boto.connect_ec2()
print(botoEC2.get_all_instances())
```

Wrap the Python script in summon:

```
summon python listEC2.py
```

`python listEC2.py` is the command that summon wraps. Once the Python program exits,
the secrets stored in temp files and in the Python process environment are gone.

### `secrets.yml` Flags

Currently, you can define how the value of a variable will be processed using YAML tags. Multiple
tags can be defined per variable by spearating them with `:`. By default, values are resolved
as literal values.
- `!file`: Resolves the variable value, places it into a tempfile, and returns the path to that
file.
- `!var`: Resolves the value as a variable ID from the provider.
- `!str`: Resolves the value as a literal (default).
- `!default='<value>'`: If the value resolution returns an empty string, use this literal value
instead for it.

> **Note on double-quoted literals and `\n`:** YAML double-quoted strings expand escape sequences
> before summon sees the value. Writing `KEY: "hello\nworld"` produces a value with a literal
> newline character, not the two-character sequence `\n`. When summon detects a double-quoted
> literal containing an embedded newline it logs a warning at the `WARN` level. Use single quotes
> (`'hello\nworld'`) if you want the backslash-n to pass through unchanged.

**Examples**
```yaml
# Resolved summon-env string (eg. `production/sentry/api_key`) is sent to the provider
# and the value returned is saved in the variable.
API_KEY: !var $env/sentry/api_key

# Resolved summon-env string (eg. `production/aws/ec2/private_key`) is sent to the provider.
# The returned value is put into a tempfile and the path for that file is saved in the
# variable.
API_KEY_PATH: !file:var $env/aws/ec2/private_key

# Literal value `my content` is saved into a tempfile and the path for that file is saved
# in the variable.
SECRET_DATA: !file my content

# Resolved summon-env string (eg. `production/sentry/api_user`) is sent to the provider.
# The returned value is put into a tempfile. If the value from the provider is an empty
# string then the default value (`admin`) is put into that tempfile. The path to that
# tempfile is saved in the variable.
API_USER: !var:default='admin':file $env/sentry/api_user
```

### Default values

Default values can be set by using the `default='yourdefaultvalue'` as an addtional tag on the variable:
```yaml
VARIABLE_WITH_DEFAULT: !var:default='defaultvalue' path/to/variable
```

### Flags

`summon` supports a number of flags.

* `-p, --provider <path-to-provider>` specify the path to the
[provider](pkg/provider/README.md) summon should use.

    If you do not provide Summon with the full path to the provider, Summon will look for providers in the following order:
    * Environment Variable: `SUMMON_PROVIDER` : set to `conjur` to explicitly select the built-in Conjur provider, or to a provider binary name to select an installed external provider
    * Environment Variable: `SUMMON_PROVIDER_PATH`
    * `/usr/local/lib/summon` on Linux / Mac
    * `%ProgramW6432%\Cyberark Conjur\Summon\Providers` on Windows.
    * `${summon binary dir}/Providers` For portable installation
    * `${summon binary dir}/../lib/summon` For homebrew installations

* `-f <path>` specify a location to a secrets.yml file, default 'secrets.yml' in current directory.

* `--up` searches for secrets.yml going up, starting from the current working
  directory.

    Stops at the first file found or when the root of the current file system is
    reached. This allows to be at any directory depth in a project and simply do
    `summon -u <command>`.

* `-D 'var=value'` causes substitution of `value` to `$var`.

    You can use the same secrets.yml file for different environments, using `-D` to
    substitute variables. This flag can be used multiple times.

    *Example*

    ```
    summon -D ENV=production --yaml 'SQL_PASSWORD: !var env/$ENV/db-password' deploy.sh
    ```

    *Warning: Never embed plaintext secret values in the subsitution string passed to this flag - command-line arguments are exposed in process listings and shell history.*

* `--yaml <YAML-string>` Passes secrets.yml as a literal string.

    This flag is used to pass a literal YAML string to the provider in place
    of the `secrets.yml` file (see example above).

    *Warning: Never embed plaintext secret values in the YAML string passed to this flag - command-line arguments are exposed in process listings and shell history.*

* `-i, --ignore <path-to-provider>` A secret path for which to ignore provider
errors.

    This flag can be useful for when you have secrets that you don't need access to for development. For example API keys for monitoring tools. This flag can be used multiple times.

* `-I, --ignore-all` A boolean to ignore any missing secret paths.

    This flag can be useful when the underlying system that's going to be using the values implements defaults. For example, when using summon as a bridge to [confd](https://github.com/kelseyhightower/confd).

* `-V, --all-provider-versions` List of all of the providers in the default
    path and their versions (if they have the --version tag).
* `-v, --version` Print the Summon version.

* `-d, --debug` Enable debug logging.

    When set, summon prints detailed log messages (at `DEBUG` level) to stderr, including configuration loading, secret fetching progress, and error details. Useful for troubleshooting provider or secrets.yml issues.

* `--refresh` (aliases `--daemon`, `--sidecar`) Keep running and pick up rotated secrets. See [Refresh mode](#refresh-mode).

* `-e, --environment` Specify section (environment) to parse from secret YAML.

    This flag specifies which specific environment/section to parse from the secrets YAML file (or string). In addition, it will also enable the usage of a `common` (or `default`) section which will be inherited by other sections/environments. In other words, if your `secrets.yaml` looks something like this:

```yaml
common:
  DB_USER: db-user
  DB_NAME: db-name
  DB_HOST: db-host.example.com

staging:
  DB_PASS: some_password

production:
  DB_PASS: other_password
```

Doing something along the lines of: `summon -f secrets.yaml -e staging printenv | grep DB_`, `summon` will populate `DB_USER`, `DB_NAME`, `DB_HOST` with values from `common` and set `DB_PASS` to `some_password`.

Note: `default` is an alias for `common` section. You can use either one.
Also note that when not using named environments, the `common` section will be ignored.

* `-h` View help and all flags.

### env-file

Using Docker? When you run summon it also exports the variables and values from secrets.yml in `VAR=VAL` format to a memory-mapped file, its path made available as `@SUMMONENVFILE`.

You can then pass secrets to your container using Docker's `--env-file` flag like so:

```sh
summon docker run --env-file @SUMMONENVFILE myorg/myimage
```

This file is created on demand - only when `@SUMMONENVFILE` appears in the
arguments of the command summon is wrapping. This feature is not Docker-specific; if you have another tools that reads variables in `VAR=VAL` format
you can use `@SUMMONENVFILE` just the same.

## Refresh mode

By default Summon fetches secrets once and exits with the subprocess. With `--refresh`
(aliases `--daemon`, `--sidecar`) it keeps running: it re-fetches every secret on an
interval while the subprocess runs.

Refresh mode is not supported on Solaris: Summon cannot track the subprocess tree there, so
`--refresh` exits with an error (`refresh mode is not supported on solaris`). One-shot mode
works as usual.

On every poll Summon compares SHA-256 checksums against the previous poll:

* **File secrets** (`summon.files`) are rewritten atomically when their rendered content
  changed, regardless of the restart setting.
* **Any change** (env-injected secret or file) restarts the subprocess with freshly fetched
  values, unless restart is disabled. A restart stops the whole process tree gracefully,
  waits up to `SUMMON_TERM_GRACE_PERIOD`, force-kills whatever is left, and relaunches
  with a fresh environment. Env-injected secrets cannot be changed in a running process,
  which is why restart is the only rotation action.

Secret files are rewritten before the restart, so for a moment the old subprocess may see
new file content alongside its old environment.

Set `SUMMON_RESTART_ON_ROTATION=false` when the application re-reads its secret files on
its own: Summon then only polls and rewrites files.

| Variable | Default | Description |
|---|---|---|
| `SUMMON_REFRESH_INTERVAL` | `5m` | Time between polls (minimum `5s`) |
| `SUMMON_REFRESH_TIMEOUT` | `60s` | Per-poll fetch timeout; exceeding it cancels the fetch (see below) and counts as a failure |
| `SUMMON_RESTART_ON_ROTATION` | `true` | Restart the subprocess when secrets change; `false` = poll and rewrite files only |
| `SUMMON_TERM_GRACE_PERIOD` | `30s` | Time a restart or shutdown waits for a graceful stop before force-killing |
| `SUMMON_MAX_REFRESH_OUTAGE` | `0` (unlimited) | Wall-clock duration (e.g. `15m`) of continuous failed polls before Summon terminates the subprocess and exits with code `2`; must be at least `SUMMON_REFRESH_INTERVAL` |
| `SUMMON_MAX_AUTH_FAILURES` | `5` | Number of consecutive polls answered with HTTP 401 before Summon terminates the subprocess and exits with code `3`; `0` disables the check; a negative or non-numeric value is a configuration error |
| `SUMMON_LOG_LEVEL` | `INFO` | `DEBUG`, `INFO`, `WARN` or `ERROR`; secret values are never logged at any level |

After a failed poll the subprocess keeps running with its last-known-good secrets and the
poll is retried with exponential backoff: after `1s`, `2s`, `4s`, ... (±10% jitter), never
longer than `SUMMON_REFRESH_INTERVAL`. A fully successful poll resets the backoff and the
outage clock. The outage is the time since the first failed poll after the last fully
successful one (startup counts as successful); once it reaches `SUMMON_MAX_REFRESH_OUTAGE`,
Summon terminates the subprocess and exits with code `2`. The limit is wall-clock: the
number of retries does not matter.

A panic raised while fetching is recovered and handled as an ordinary failed poll (logged at
`WARN` without a stack, which is logged at `DEBUG` only), so a crash in a provider never orphans
the subprocess.

#### Host API key rotation

If the host's API key is rotated while Summon runs, polls keep working on the cached access
token until it expires (several minutes). After that the Conjur client logs in again with the
old key and every poll is answered with HTTP 401. Summon keeps the subprocess running on the
last-known-good secrets and retries with backoff, counting consecutive 401 responses (a
successful poll, or a failure that is not a 401 such as 403, 404 or a network error, resets the
count). After `SUMMON_MAX_AUTH_FAILURES` of them it logs an error, stops the subprocess and
exits with code `3`, so a supervisor (systemd, Kubernetes, ...) can restart Summon with the new
credentials. Every failed poll is a failed login on the server, so a Conjur lockout policy
may lock the host out earlier than the default of `5`; lower the value if needed. The check
applies to the built-in Conjur provider and to polls only; a 401 at startup fails the start as
in one-shot mode.

A fetch that runs past `SUMMON_REFRESH_TIMEOUT` is cancelled and fails the poll with
`fetch exceeded <timeout>`; the next poll starts a fresh fetch. The same cancellation aborts
an in-flight fetch when Summon receives `SIGINT`/`SIGTERM`, so shutdown does not wait for it.
What cancellation does:

* External provider: the provider process is killed (on Unix its whole process group, so
  helpers it started die with it) and Summon stops waiting for its output within seconds,
  even if a process that left the group still holds the pipes. On Windows only the provider
  process itself is killed.
* Built-in Conjur provider: the secret request is aborted (connect, wait for the response and
  body read). Authentication, which the Conjur client may perform before the request when
  its access token expires, cannot be cancelled through a context; it is bounded by the
  client's HTTP timeout, which in daemon mode is lowered to `SUMMON_REFRESH_TIMEOUT` (rounded
  up to whole seconds) when that is shorter than `CONJUR_HTTP_TIMEOUT` / the 60s default.
  The built-in provider also stops before fetching the next secret once the context is done.
  The lowered limit applies to every request of the run, including the startup fetch.

A poll that returns an empty value for a secret whose last-known-good value was non-empty
(env or file secret, any provider) is treated as a failed poll: Summon logs a warning naming
the variable, keeps the last-known-good value of every secret (no file rewrite, no restart)
and recovers on the first poll that returns a non-empty value. A secret that was empty at
startup stays valid.

When Summon itself receives `SIGINT` or `SIGTERM`, it stops the subprocess tree like a
restart does (graceful stop, up to `SUMMON_TERM_GRACE_PERIOD`, then force-kill of whatever is
left) and exits with the subprocess's exit code, or `0` if the subprocess was ended by the
stop signal or the force-kill. A second `SIGINT`/`SIGTERM` force-kills immediately.

Exit codes: the subprocess's own exit code when it exits; `2` when
`SUMMON_MAX_REFRESH_OUTAGE` is reached; `3` when `SUMMON_MAX_AUTH_FAILURES` is reached;
`127` for setup or configuration errors. Whichever of the two limits is reached first wins.

Notes:

* With an external provider, `CONJUR_*` variables stay in Summon's own environment for the
  whole run because the provider is re-invoked on every poll. They are never passed to the
  subprocess.
* Signals: Summon forwards every signal it receives to the subprocess except `SIGURG` and
  `SIGCHLD`; `SIGINT` and `SIGTERM` start the shutdown described above instead of being
  forwarded as-is. `SIGHUP` is only forwarded: it neither stops Summon nor arms the force-kill. Any process of the same user that can signal Summon (for example `SIGHUP` or
  `SIGUSR1`) therefore reaches the application.
* `--ignore` / `--ignore-all` apply to the startup fetch only. On later polls an ignored
  secret that failed at startup and was ignored keeps that state for the whole run: it is not
  fetched again (it never counts as a failed poll and is never added later, even if the backend
  recovers; restart Summon to pick it up). A secret fetched at startup that fails on a later
  poll counts as a failed poll and the last-known-good secrets are kept.
* Hardening: Summon exits with `127` if it cannot disable core dumps, and on Linux marks
  itself non-dumpable (`PR_SET_DUMPABLE=0`). `os.Unsetenv` does not scrub the kernel's copy
  of the initial environment, so `CONJUR_*` values passed in the environment remain visible
  in `/proc/<pid>/environ`; being non-dumpable makes that file readable by root only.
  On other Unix systems core dumps are disabled with `RLIMIT_CORE` and the provider and
  push-to-file checks below run, but the process is not marked non-dumpable; Summon prints
  one warning line to stderr at `--refresh` startup saying so (shown even with
  `SUMMON_LOG_LEVEL=ERROR`). On Windows `--refresh` runs without core-dump suppression,
  provider binary verification and push-to-file directory checks, and prints one warning
  line saying so at startup. Linux prints no warning. One-shot runs print none of these.
* An external provider binary and its directory must not be group- or world-writable, and
  the binary is re-checked (owner, mode, device/inode, mtime) before every invocation; a
  changed binary fails the poll. Provider stderr is logged only at `DEBUG`.
* Push-to-file destinations must be in a directory owned by the Summon user or root and
  not group- or world-writable, and every ancestor directory (after resolving symlinks)
  must be owned by the Summon user or root and not group- or world-writable, except a
  sticky root-owned directory such as `/tmp`. A destination that is a symlink is
  refused. The checks run before every rewrite, and the directory is fsynced after each
  rewrite. (Unix only.)

### Windows

Restarts work on Windows with these limits:

* The subprocess tree is tracked with a Job Object and stopped with `CTRL_BREAK`, which
  reaches console processes that share Summon's console. The workload must handle
  `CTRL_BREAK` (for example `os.Interrupt` in Go). Anything that does not exit within the
  grace period is terminated together with its descendants.
* The subprocess runs in a new process group and therefore ignores Ctrl+C; Summon forwards
  its own interrupt as `CTRL_BREAK`.
* Services, GUI applications and processes without a shared console get no graceful stop
  and are force-killed when the grace period expires.
* If Summon itself has no console (for example when it runs as a Windows service), every
  restart force-kills immediately; Summon logs a warning at startup.

## Trust Model

Summon's responsibilities - fetching secrets from a provider, and either
injecting those secrets into a subprocess' environment or writing them
temporarily to the filesystem - are security critical, and therefore editing the
contents of a `secrets.yml` and the invocation of it via Summon must be
considered privileged behavior. Changes to `secrets.yml` must not be made
without extremely scrupulous security review, and Summon should not be allowed
to be invoked by untrusted parties or in untrusted environments.

### Core dumps (Linux)

Core dumps can expose secret values and access tokens held in process memory.
For production environments, disable core dumps at the deployment layer
(for example `ulimit -c 0` in the shell, or `LimitCORE=0` in a systemd unit).

On Linux, Summon also calls `Setrlimit(RLIMIT_CORE, 0)` at startup as
defense-in-depth. If that call fails (for example `EPERM` in a rootless
container), Summon logs a debug-level warning when `-d` / `--debug` is set and
continues normally.

## Push-to-File

`summon.files` lets you write resolved secrets directly to files rather than environment
variables. Summon fetches each secret via the provider and renders a file from a Go
`text/template` (or a built-in format), then writes the result atomically with the
configured permissions. The file will be removed when the summon process exits.

### Configuration fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `path` | string | Yes | : | Destination file path (absolute or relative to working directory) |
| `format` | string | No | `yaml` | Output format; see table below |
| `template` | string | No | : | Inline Go `text/template` string; required when `format: template` |
| `permissions` | octal | No | `0600` | File permission bits |
| `overwrite` | bool | No | `false` | Overwrite the file if it already exists |
| `secrets` | mapping | Yes | : | Map of alias → `!var` / `!str` secret references |

### Supported `format:` values

| Value | Description |
|---|---|
| `yaml` | JSON-compatible YAML mapping of `"alias": "value"` pairs **(default)** |
| `json` | JSON object of `"alias": "value"` pairs |
| `dotenv` | `ALIAS=value` lines suitable for `.env` files |
| `properties` | `alias=value` Java-style properties |
| `bash` | `export ALIAS=value` lines |
| `template` | Inline Go `text/template` string supplied in the `template:` field |

### Template rendering context

When `format: template` is used, the following functions and variables are available:

| Symbol | Description |
|---|---|
| `secret "alias"` | Returns the resolved value for the given alias |
| `b64enc` | Base64-encodes a string |
| `b64dec` | Base64-decodes a string; errors if the input is not valid base64 |
| `htmlenc` | HTML-encodes a string |
| `propertiesenc` | Escapes a string for Java `.properties` value syntax |
| `.SecretsArray` | `[]Secret` : all secrets sorted lexicographically by alias |
| `.SecretsMap` | `map[string]Secret` : all secrets keyed by alias |

All built-in Go `text/template` functions (e.g. `html`, `urlquery`, `printf`) are also
available.

### Example

```yaml
# secrets.yml
summon.files:
  - path: "./config/db.conf"
    format: template
    permissions: 0640
    overwrite: true
    template: |
      [database]
      host     = {{ secret "DB_HOST" }}
      username = {{ secret "DB_USERNAME" }}
      password = {{ secret "DB_PASSWORD" }}
      api_key  = {{ secret "API_KEY" | b64enc }}
    secrets:
      DB_HOST:     !var app/prod/db-host
      DB_USERNAME: !var app/prod/db-username
      DB_PASSWORD: !var app/prod/db-password
      API_KEY:     !var app/prod/api-key
```

Running `summon -p <provider> cat ./config/db.conf` will write `config/db.conf` with the
resolved secrets substituted into the template, followed by outputting its contents to
STDOUT, and finally the file will be removed when the summon process exits.

### Security considerations

* **File cleanup is not guaranteed.** Summon removes pushed files when the
  wrapped process exits normally or is terminated by a signal it can catch.
  However, if the summon process is forcefully killed (e.g. `SIGKILL` /
  `kill -9`) or the system crashes, the files may remain on disk. You should
  have a secondary cleanup mechanism (e.g. a startup script, `tmpwatch`, or
  an ephemeral filesystem) for environments where this is a concern.
* **Use the most restrictive permissions possible.** The default file
  permissions are `0600` (owner read/write only). If your application allows
  it, keep the default. Avoid overly permissive modes such
  as `0644` or `0755` which expose secret files to other users on the system.

---

## Fixed tempfile name
There are times when you would like to have certain secrets values available at
fixed locations, e.g. `/etc/ssl/cert.pem` for an SSL certificate. This can be
accomplished by using symbolic links as described in the
[symbolic link example](examples/symlinks/README.md).

## Provider interactive mode

When available, Summon uses the provider's stream mode to retrieve secrets. Whereas the legacy mode required
a new process to be created for each secret retrieval, the stream mode can fetch multiple secrets in a single
process and allows providers to implement token caching.

If the provider does not support stream mode, Summon uses the legacy mode.

### Timeout

By default, Summon waits up to 60 seconds for an external provider binary to
finish (both legacy one-shot `Call` and stream/interactive mode). Override with
`SUMMON_PROVIDER_TIMEOUT` using a Go duration string (for example `30s` or `2m`).

This limit applies to provider **binaries** only. It is separate from
`CONJUR_HTTP_TIMEOUT`, which the built-in Conjur provider uses (via conjur-api-go)
as an HTTP client timeout.

For stream/interactive mode only, if `SUMMON_PROVIDER_TIMEOUT` is unset, Summon
still honors `CONJUR_HTTP_TIMEOUT` as a deprecated fallback (positive integer
seconds). Prefer `SUMMON_PROVIDER_TIMEOUT` for new configurations.

## Providers

Summon fetches secrets through a provider binary. The built-in Conjur provider is included
with Summon and requires no separate installation — see
[Using Summon with CyberArk Secrets Manager](#using-summon-with-cyberark-secrets-manager) above.

Additional community providers are available:

- [AWS S3](https://github.com/conjurinc/summon-s3)
- [AWS Secrets Manager](https://github.com/cyberark/summon-aws-secrets)
- [Chef encrypted data bags](https://github.com/conjurinc/summon-chefapi)
- [keyring](https://github.com/conjurinc/summon-keyring)
- [Keepass kdbx](https://github.com/mskarbek/summon-keepass)
- [Gopass](https://github.com/gopasspw/gopass-summon-provider)

## Contributing

For more info on contributing, please see [CONTRIBUTING.md](CONTRIBUTING.md).

## Troubleshooting

For assistance with some issues encountered when first using Summon, please refer to the
[troubleshooting guide](CONTRIBUTING.md#Troubleshooting) in 
[CONTRIBUTING.md](CONTRIBUTING.md).

Can't find your problem in the troubleshooting guide? [File an issue](https://github.com/cyberark/summon/issues/new/choose)
or [ask us on Discourse](https://discuss.cyberarkcommons.org/c/summon/30).

## License

Copyright (c) 2020 CyberArk Software Ltd. All rights reserved.

Summon is available under the [MIT License](LICENSE).
