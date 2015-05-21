# cauldron

cauldron provides an interface for

* reading the **secrets.yml** format
* fetching secretsfrom a trusted store
* exporting their values to the environment

Parse secrets.yml to export environment variables fetched from a trusted store.

## secrets.yml

secrets.yml defines a format for mapping environment variables to locations of
secrets.

```yml
AWS_ACCESS_KEY_ID: aws/iam/user/robot/access_key_id
AWS_PEM: !file aws/iam/user/robot/pem_file
```

Running an implementation of cauldron against this example file will fetch the secret
defined at `aws/iam/user/robot/access_key_id` in a secrets server and set the environment
variable `AWS_ACCESS_KEY_ID` to the secret's value. The value of `AWS_PEM` will be the
path to a temporary file that is cleaned up on exit of the process cauldron is wrapping.

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
cauldron-myprovider run python listEC2.py
```

`python listEC2.py` is the command that cauldron wraps. Once this command exits
and secrets in the environment are gone.

### Flags

`cauldron run` supports a number of flags.

**`-f <path>`** specify a location to a secrets.yml file.

```
cauldron run -f /etc/mysecrets.yml
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
$ cauldron-myprovider -D '$environment=development' env | grep AWS
AWS_ACCESS_KEY_ID=mydevelopmentkey
```

**`-- yaml 'key:value'`** secrets.yml as a literal string

A string in secrets.yml format can also be passed to cauldron.

```sh-session
$ cauldron-myprovider --yaml 'MONGODB_PASS: db/dbname/password' chef-apply
```

This will make `ENV['MONGODB_PASS']` available in your Chef run.

View help and all flags with `cauldron -h`.

## Adding Providers

Adding a provider to cauldron is easy. All you have to do is point cauldron to a program that takes a secret's location as an
argument and return the secret's value.

TODO: expand on this

## Development

Install dependencies

```
xargs -L1 go get <Godeps
```

Run the project with `go run *.go`.

Run tests with `go test ./...` or `./test.sh` (for CI).

## Building

To build 64bit versions for Linux, OSX and Windows:

```
./build.sh
```

Binaries will be placed in `pkg/`.
