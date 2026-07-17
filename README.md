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

##### Authentication

| Variable | Description |
|---|---|
| `CONJUR_AUTHN_LOGIN` | Conjur identity (user or host) |
| `CONJUR_AUTHN_API_KEY` | API key for the identity |
| `CONJUR_AUTHN_TOKEN` | Pre-fetched Conjur auth token (inline) |
| `CONJUR_AUTHN_TOKEN_FILE` | Path to a file containing the auth token |
| `CONJUR_AUTHN_JWT_SERVICE_ID` | JWT authenticator service ID (e.g. `kubernetes`) |
| `JWT_TOKEN_PATH` | Path to the JWT token file (default: `/var/run/secrets/kubernetes.io/serviceaccount/token`) |
| `CONJUR_AUTHN_TYPE` | Auth backend: `iam`, `azure`, or `gcp` |
| `CONJUR_SERVICE_ID` | Authenticator service ID for IAM and Azure |
| `CONJUR_AUTHN_JWT_HOST_ID` | Conjur host identity for IAM, Azure, and JWT auth; for GCP, required only when auto-fetching the token from the GCP metadata service |
| `CONJUR_AUTHN_AZURE_CLIENT_ID` | Azure managed identity client ID (optional; selects a specific identity on multi-identity hosts) |
| `CONJUR_AUTHN_JWT_TOKEN` | Inline JWT token string (alternative to `JWT_TOKEN_PATH`; use when the token is available as a value rather than a file) |

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
