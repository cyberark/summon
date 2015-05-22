# cauldron

cauldron provides an interface for

* reading the **secrets.yml** format
* fetching secrets from a trusted store
* exporting their values to the environment

Parse secrets.yml to export environment variables fetched from a trusted store.

## secrets.yml

secrets.yml defines a format for mapping environment variables to locations of
secrets.

```yml
AWS_ACCESS_KEY_ID: !var /aws/iam/user/robot/access_key_id
AWS_PEM: !file /aws/iam/user/robot/pem_file
ENVIRONMENT: $environment
```

Running an implementation of cauldron against this example file will

1. Fetch the secret defined at `aws/iam/user/robot/access_key_id` in a secrets server and set the environment variable `AWS_ACCESS_KEY_ID` to the secret's value.
2. The value of `AWS_PEM` will be the path to a temporary memory-mapped file that is cleaned up on exit of the process cauldron is wrapping.
3. `ENVIRONMENT` will be interpolated at runtime by using cauldron's `-D` flag, like so: `cauldron -D '$environment=production'`. This flag can be specified multiple times.


## Providers

Cauldron uses plug-and-play providers. A provider is any executable that satisfies this contract:

* Accepts one argument, where a secret is located
* Returns the value of the secret on stdout and exit code 0 if retrieval was successful.
* Returns an error message on stderr and a non-0 exit code if retrieval was unsuccessful.

Providers can be written in any language you prefer. They can be as simple as a shell script.

When cauldron runs it will look for a provider in `/usr/libexec/cauldron/`. If there is one executable
in this directory it will use it as the default provider. If the directory hold multiple executables
you will be prompted to select the one you want to use. You can set the provider with the `-p, --provider`
flag to the CLI or via the environment variable `CAULDRON_PROVIDER`. If your provider is in a location
not on your `PATH` you will need to specify its full path.

For example, if you have multiple providers and want to use the one for [vault](https://vaultproject.io/), this is your command.

```sh-session
$ cauldron --provider /usr/libexec/cauldron/vault
```

## Usage

By default, cauldron will look for `secrets.yml` in the directory it is
called from and export the secret values to the environment of the command it wraps.

*Example*

You want to run script that requires AWS keys to list your EC2 instances.

Define your keys in a `secrets.yml` file

```yml
AWS_ACCESS_KEY_ID: aws/iam/user/robot/access_key_id
AWS_SECRET_ACCESS_KEY: aws/iam/user/robot/secret_access_key
```

The script uses the Python library [boto](https://pypi.python.org/pypi/boto), which looks for `AWS_ACCESS_KEY_ID`
and `AWS_SECRET_ACCESS_KEY` in the environment.

```python
import boto
botoEC2 = boto.connect_ec2()
print(botoEC2.get_all_instances())
```

Wrap running this script in cauldron.

```
cauldron python listEC2.py
```

`python listEC2.py` is the command that cauldron wraps. Once this command exits
and secrets in the environment are gone.

### Flags

`cauldron` supports a number of flags.

**`-f <path>`** specify a location to a secrets.yml file.

```
cauldron -f /etc/mysecrets.yml
```

**`-D '$var=value'`** causes substitution of `value` to `$var`.

You can use the same secrets.yml file for different environments, using `-D` to
substitute variables.

*Example*

secrets.yml
```yml
AWS_ACCESS_KEY_ID: $environment/aws/iam/user/robot/access_key_id
```

```sh-session
$ cauldron -D '$environment=development' env | grep AWS
AWS_ACCESS_KEY_ID=mydevelopmentkey
```

**`-- yaml 'key:value'`** secrets.yml as a literal string

A string in secrets.yml format can also be passed to cauldron.

```sh-session
$ cauldron --yaml 'MONGODB_PASS: db/dbname/password' chef-apply
```

This will make `ENV['MONGODB_PASS']` available in your Chef run.

View help and all flags with `cauldron -h`.

## Development

Install dependencies

```
xargs -L1 go get <Godeps
```

Run the project with `go run *.go`.

### Testing

Tests are written using [GoConvey](http://goconvey.co/).
Run tests with `go test -v ./...` or `./test.sh` (for CI).

### Building

To build 64bit versions for Linux, OSX and Windows:

```
./build.sh
```

Binaries will be placed in `pkg/`.
