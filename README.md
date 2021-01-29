# summon

<div align="center">
  <a href="https://cyberark.github.io/summon">
    <img src="https://cyberark.github.io/summon/images/logo.png" height="200"/><br>
    cyberark.github.io/summon
  </a>
</div>

[![GitHub release](https://img.shields.io/github/release/cyberark/summon.svg)](https://github.com/cyberark/summon/releases/latest)

[![Github commits (since latest release)](https://img.shields.io/github/commits-since/cyberark/summon/latest.svg)](https://github.com/cyberark/summon/commits/master)

---

`summon` is a command-line tool to make working with secrets easier.

It provides an interface for

* Reading a `secrets.yml` file
* Fetching secrets from a trusted store
* Exporting secret values to a sub-process environment

## Install

Note installing `summon` alone is not sufficient; you need to also install
a [provider of your choice](http://cyberark.github.io/summon/#providers) before it's ready for use.

Pre-built binaries and packages are available from GitHub releases
[here](https://github.com/cyberark/summon/releases).

### Using Summon with Conjur OSS 

Are you using this project with [Conjur OSS](https://github.com/cyberark/conjur)? Then we 
**strongly** recommend choosing the version of this project to use from the latest [Conjur OSS 
suite release](https://docs.conjur.org/Latest/en/Content/Overview/Conjur-OSS-Suite-Overview.html). 
Conjur maintainers perform additional testing on the suite release versions to ensure 
compatibility. When possible, upgrade your Conjur version to match the 
[latest suite release](https://docs.conjur.org/Latest/en/Content/ReleaseNotes/ConjurOSS-suite-RN.htm); 
when using integrations, choose the latest suite release that matches your Conjur version. For any 
questions, please contact us on [Discourse](https://discuss.cyberarkcommons.org/c/conjur/5).

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
appropriate release to ensure support for your version of Conjur.

Use the auto-install script. This will install the latest version of summon.
The script requires sudo to place summon in `/usr/local/bin`.

```
curl -sSL https://raw.githubusercontent.com/cyberark/summon/master/install.sh | bash
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

* `-p, --provider` specify the path to the [provider](provider/README.md) summon should use.

    If the provider is in the default path, `/usr/local/lib/summon/` (or
    `%ProgramW6432%\Cyberark Conjur\Summon\Providers` on Windows) you can just
    provide the name of the executable. If not, use a full path.

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

* `--yaml` secrets.yml as a literal string.

    This flag is used to pass `secrets.yml` to the provider as a literal string
    (see example above).

* `-i, --ignore` A secret path for which to ignore provider errors.

    This flag can be useful for when you have secrets that you don't need access to for development. For example API keys for monitoring tools. This flag can be used multiple times.

* `-I, --ignore-all` A boolean to ignore any missing secret paths.

    This flag can be useful when the underlying system that's going to be using the values implements defaults. For example, when using summon as a bridge to [confd](https://github.com/kelseyhightower/confd).

* `-V, --all-provider-versions` List of all of the providers in the default
    path and their versions (if they have the --version tag).
* `-v, --version` Print the Summon version.

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
