# summon

[conjurinc.github.io/summon](https://conjurinc.github.io/summon)

`summon` provides an interface for

* Reading a **secrets.yml** file
* Fetching secrets from a trusted store
* Exporting secret values to a sub-process environment

Note that summon is still in **early stages**, we are looking for feedback and contributions.

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

* `-p, --provider` specify the path to the provider summon should use

    If the provider is in the default path, `/usr/libexec/summon/` you can just 
    provide the name of the executable. If not, use the full path.

* `-f <path>` specify a location to a secrets.yml file, default 'secrets.yml' in current directory.

* `-D '$var=value'` causes substitution of `value` to `$var`.

    You can use the same secrets.yml file for different environments, using `-D` to
    substitute variables. This flag can be used multiple times.

* `-i, --ignore` A secret path for which to ignore provider errors

    This flag can be useful for when you have secrets that you don't need access to for development. For example API keys for monitoring tools. This flag can be used multiple times.

View help and all flags with `summon -h`.

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
