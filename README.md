[![build status](https://img.shields.io/circleci/project/github/kumbirai-logistics/summon.svg)](https://circleci.com/gh/kumbirai-logistics/workflows/summon)

# summon

<div align="center">
  <a href="https://cyberark.github.io/summon">
    <img src="https://cyberark.github.io/summon/images/logo.png" height="200"/><br>
    cyberark.github.io/summon
  </a>
</div>

---

`summon` is a command-line tool to make working with secrets easier.

It provides an interface for

* Reading a **secrets.yml** file
* Fetching secrets from a trusted store
* Exporting secret values to a sub-process environment

Note that summon is still in **early stages**, we are looking for feedback and contributions.

## Install

Note basic **summon** install is not fully functional; you need to also install a [provider of your choice](http://cyberark.github.io/summon/#providers) before it's ready for use.

### OSX

Install via [Homebrew](http://brew.sh/).

```sh
brew tap cyberark/tools
brew install summon
```

### Linux

Use the auto-install script. This will install the latest version of summon.
The script requires sudo to place summon in `/usr/local/bin`.

```
curl -sSL https://raw.githubusercontent.com/cyberark/summon/master/install.sh | bash
```

For other platforms, download the [latest release](https://github.com/cyberark/summon/releases/latest)
and unzip it to a location on your PATH.

## Usage

By default, summon will look for `secrets.yml` in the directory it is
called from and export the secret values to the environment of the command it wraps.

*Example*

You want to run script that requires AWS keys to list your EC2 instances.

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

### Flags

`summon` supports a number of flags.

* `-p, --provider` specify the path to the [provider](provider/README.md) summon should use

    If the provider is in the default path, `/usr/local/lib/summon/` you can just
    provide the name of the executable. If not, use a full path.

* `-f <path>` specify a location to a secrets.yml file, default 'secrets.yml' in current directory.

* `-D 'var=value'` causes substitution of `value` to `$var`.

    You can use the same secrets.yml file for different environments, using `-D` to
    substitute variables. This flag can be used multiple times.

    *Example*

    ```
    summon -D ENV=production --yaml 'SQL_PASSWORD: !var env/$ENV/db-password' deploy.sh
    ```

* `-i, --ignore` A secret path for which to ignore provider errors

    This flag can be useful for when you have secrets that you don't need access to for development. For example API keys for monitoring tools. This flag can be used multiple times.

* `-e, --environment` Specify section (environment) to parse from secret YAML

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

View help and all flags with `summon -h`.

### env-file

Using Docker? When you run summon it also exports the variables and values from secrets.yml in `VAR=VAL` format to a memory-mapped file, its path made available as `@SUMMONENVFILE`.

You can then pass secrets to your container using Docker's `--env-file` flag like so:

```sh
summon docker run myorg/myimage --env-file @SUMMONENVFILE
```

This file is created on demand - only when `@SUMMONENVFILE` appears in the
arguments of the command summon is wrapping. This feature is not Docker-specific; if you have another tools that reads variables in `VAR=VAL` format
you can use `@SUMMONENVFILE` just the same.

## Development

Dependencies are vendored with [godep](https://github.com/tools/godep).
To make them available, run `export GOPATH=`godep path`:$GOPATH`.

Run the project with:

```
go run *.go`.
```

### Testing

Tests are written using [GoConvey](http://goconvey.co/).
Run tests with `go test -v ./...` or `./test.sh` (for CI).

### Building

To build 64bit versions for Linux, OSX and Windows:

```
./build.sh
```

Binaries will be placed in `output/`.

### Packaging

```
./package.sh
```
