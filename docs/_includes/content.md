# overview

summon is a command-line tool that reads a file in secrets.yml format
and injects secrets as environment variables into any process. It can also
write secrets directly to files in common formats like JSON, YAML, and dotenv,
or render them with custom Go templates.
Once the process exits, the secrets are gone.

<div style="text-align: center">
  <img src="//i.imgur.com/ZeSpdZT.png" width="80%" />
</div>

summon is not tied to a particular secrets source. Instead, sources are implemented as providers
that summon calls to fetch values for secrets. Providers need only satisfy a simple contract
and can be written in any language.

Running summon looks like this:

```bash
summon --provider conjur -f secrets.yml chef-client --once
```

summon resolves the entries in `secrets.yml` with the `conjur` provider and
makes the secret values available to the environment of the command `chef-client --once`.
In our chef recipes we can access the secrets with Ruby's `ENV['...']` syntax.

This same pattern works for any tooling that can access environment variables.

As a second example, Docker:

```bash
summon --provider conjur -f secrets.yml docker run --env-file @SUMMONENVFILE myapp
```

Full usage docs for summon are in the
[Github README for the project](https://github.com/cyberark/summon).

<h2 id="secrets.yml">secrets.yml</h2>

secrets.yml defines a format for mapping an environment variable to a location
where a secret is stored. There are no sensitive values in this file itself. It can safely be checked into source control. Given a secrets.yml file, summon fetches the values
of the secrets from a provider and provide them as environment variables
for a specified process.

The format is basic YAML with an optional tag. Each line looks like this:

```
<key>: !<tag> <secret>
```

`key` is the name of the environment variable you wish to set.

`tag` sets a context for interpretation:

* `!var` the value of `key` is set to the the secret's value, resolved by a provider given `secret`.

* `!file` writes the literal value of `secret` to a memory-mapped temporary
file and sets the value of `key` to the file's path.

* `!var:file` is a combination of the two. It will use a provider to fetch the value of a secret
identified by `secret`, write it to a temp file and set `key` to the temp file path.

* If there is no tag, `<secret>` is treated as a literal string and set as the value of `key`.
In this scenario, the value in the `<secret>` should not actually be a secret, but rather a piece of
metadata which is associated with secrets.

Here is an example:

```yaml
AWS_ACCESS_KEY_ID: !var aws/$environment/iam/user/robot/access_key_id
AWS_SECRET_ACCESS_KEY: !var aws/$environment/iam/user/robot/secret_access_key
AWS_REGION: us-east-1
SSL_CERT: !var:file ssl/certs/private
```

`$environment` is an example of a substitution variable, given as an flag argument when running summon.

<h1 id="push-to-file">push to file</h1>

In addition to exporting secrets as environment variables, summon can write
resolved secrets directly to files using the `summon.files` key in secrets.yml.
The files are written atomically with the configured permissions and are
automatically removed when the summon process exits.

## Configuration

Add a `summon.files` list to your secrets.yml. Each entry describes one output
file:

```yaml
summon.files:
  - path: "./config/db-secrets.json"
    format: "json"
    secrets:
      DB_USERNAME: !var app/db/username
      DB_PASSWORD: !var app/db/password
```

Each file entry supports the following fields:

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `path` | string | Yes | — | Destination file path (absolute or relative to working directory) |
| `format` | string | No | `yaml` | Output format (see table below) |
| `template` | string | No | — | Inline Go `text/template` string; required when `format: template` |
| `permissions` | octal | No | `0600` | File permission bits |
| `overwrite` | bool | No | `false` | Overwrite the file if it already exists |
| `secrets` | mapping | Yes | — | Map of alias → `!var` / `!str` secret references |

## Supported formats

| Value | Description |
|---|---|
| `yaml` | YAML mapping of `"alias": "value"` pairs **(default)** |
| `json` | JSON object of `"alias": "value"` pairs |
| `dotenv` | `ALIAS=value` lines suitable for `.env` files |
| `properties` | `ALIAS=value` Java-style properties |
| `bash` | `export ALIAS=value` lines |
| `template` | Use the inline Go `text/template` from the `template:` field |

## Custom templates

When `format: template` is used, the
following functions and variables are available inside the template:

| Symbol | Description |
|---|---|
| `secret "alias"` | Returns the resolved value for the given alias |
| `b64enc` | Base64-encodes a string |
| `b64dec` | Base64-decodes a string; errors if the input is not valid base64 |
| `htmlenc` | HTML-encodes a string |
| `.SecretsArray` | `[]Secret` — all secrets sorted lexicographically by alias |
| `.SecretsMap` | `map[string]Secret` — keyed by alias |

All built-in Go `text/template` functions (e.g. `printf`, `urlquery`) are also
available.

Example with a custom template:

```yaml
summon.files:
  - path: "./config/app.cfg"
    format: template
    template: |
      [database]
      username={{ secret "DB_USERNAME" }}
      password={{ secret "DB_PASSWORD" }}
    secrets:
      DB_USERNAME: !var app/db/username
      DB_PASSWORD: !var app/db/password
```

## Mixing environment and file secrets

You can define regular environment-variable secrets and `summon.files` entries
in the same secrets.yml file:

```yaml
DB_HOST: !var app/db/host
summon.files:
  - path: "./config/db-creds.json"
    format: json
    secrets:
      DB_USERNAME: !var app/db/username
      DB_PASSWORD: !var app/db/password
```

Running `summon -p <provider> myapp` makes `DB_HOST` available as an
environment variable while `./config/db-creds.json` is written to disk.

## Security considerations

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

<h1 id="examples">examples</h1>

Summon is meant to work with your existing toolchains. If you can access environment variables, you can use Summon.

Here are some specific examples of how you can use summon with your current tools.

* [Docker](http://cyberark.github.io/summon/docker.html)
* [Buildkite](https://github.com/angaza/summon-buildkite-plugin)

<h1 id="providers">providers</h1>

Developed by Cyberark:

* [AWS S3](https://github.com/conjurinc/summon-s3)
* [Conjur](https://github.com/cyberark/summon-conjur)
* [Chef encrypted data bags](https://github.com/conjurinc/summon-chefapi)
* [keyring](https://github.com/conjurinc/summon-keyring)
* [AWS Secrets Manager](https://github.com/cyberark/summon-aws-secrets)

Developed by the community:

* [Keepass kdbx database file](https://github.com/mskarbek/summon-keepass)
* [Gopass](https://github.com/gopasspw/gopass-summon-provider)
* [KeePass 2](https://github.com/stanislavbebej-ext43345/summon-keepass)
* [Bitwarden Secrets Manager](https://github.com/stanislavbebej-ext43345/summon-secrets-manager)

Providers are easy to write. Given the identifier of a secret, they either return its value or an error.

This is their contract:

* They take one argument, the identifier of a secret (a string).
* If retrieval is successful, they return the value on stdout with exit code 0.
* If an error occurs, they return an error message on stderr and a non-0 exit code.

When providers support stream mode and a call is made without arguments, Summon continuously sends
secret identifiers to the provider's standard input, and the provider sends the secret values to its
standard output until all secrets are retrieved. The returned values are Base64 encoded to avoid issues with
special characters.

Summon always tries to use stream mode. However, when this mode is not supported Summon falls back
to the legacy mode where each secret is retrieved using its own process.

The default path for providers is `/usr/local/lib/summon/`. If one provider is in that path,
summon will use it. If multiple providers are in the path, you can specify which one to use
with the `--provider` flag or the environment variable `SUMMON_PROVIDER`. If your providers are
placed outside the default path, give summon the full path to them.

[Open a Github issue](https://github.com/cyberark/summon/issues) if you'd like to include your provider on this page.
