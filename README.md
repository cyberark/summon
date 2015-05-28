# cauldron

http://conjurinc.github.io/cauldron/

`cauldron` provides an interface for

* Reading a **secrets.yml** file
* Fetching secrets from a trusted store
* Exporting secret values to a sub-process environment
```

## Usage

By default, cauldron will look for `secrets.yml` in the directory it is
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

Wrap the Python script in cauldron:

```
cauldron python listEC2.py
```

`python listEC2.py` is the command that cauldron wraps. Once the Python program exits,
the secrets stored in temp files and in the Python process environment are gone.

### Flags

`cauldron` supports a number of flags.

**`-f <path>`** specify a location to a secrets.yml file.

```
cauldron -f /etc/mysecrets.yml
```

**`-D '$var=value'`** causes substitution of `value` to `$var`.

You can use the same secrets.yml file for different environments, using `-D` to
substitute variables.

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
