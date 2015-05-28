# overview

cauldron is a command-line tool that reads a file in secrets.yml format
and injects secrets as environment variables into any process. Once the
process exits, the secrets are gone.

<div style="text-align: center">
<img src="//i.imgur.com/WkYo2wY.png"/>
</div>

cauldron is not tied a particular secrets source. Instead, sources are implemented as providers
that cauldron calls to fetch values for secrets. Providers need only satisfy a simple contract
and can be written in any language.

Running cauldron looks like this:

```bash
cauldron --provider conjur -f secrets.yml chef-client --once
```

Cauldron resolves the entries in `secrets.yml` with the `conjur` provider and
makes the secret values available to the environment of the command `chef-client --once`.
In our chef recipes we can access the secrets with Ruby's `ENV['...']` syntax.

This same pattern works for any tooling that can access environment variables.

Full usage docs for cauldron are in the
[Github README for the project](https://github.com/conjurinc/cauldron).

## secrets.yml

secrets.yml defines a format for mapping an environment variable to a location
where a secret is stored. There are no sensitive values in this file itself. It can safely be checked into source control. Given a secrets.yml file, cauldron fetches the values
of the secrets from a provider and provide them as environment variables
for a specified process.

The format is basic YAML two custom tags. Each line looks like this:

```
<key>: !<tag> <path>
```

`key` is the name of the environment variable you wish to set.

`tag` sets a context for interpretation:

* `!var` sets the value of `key` to the the secret's value, resolved by a provider given `path`.

* `!file` writes the value of `path`, a literal string, to a memory-mapped temporary
file and sets the value of `key` to the file's path.

* `!var:file` is a combination of the two. It will use a provider to fetch the value of a secret
located at `path`, write it to a temp file and set `key` to the file's path.

* If there is no tag, `<path>` is treated as a literal string and set as the value of `key`.

Here is an example:

```yaml
AWS_ACCESS_KEY_ID: !var aws/$environment/iam/user/robot/access_key_id
AWS_SECRET_ACCESS_KEY: !var aws/$environment/iam/user/robot/secret_access_key
AWS_REGION: us-east-1
SSL_CERT: !var:file ssl/certs/private
```

`$environment` is an example of a variable, given as an flag argument when running cauldron.

# providers

<i id="providerList"></i>

* [conjurcli](https://github.com/conjurinc/cauldron-conjurcli) - Conjur CLI (for backwards compatibility)
* [osxkeychain](https://github.com/conjurinc/cauldron-keychain-cli) - OSX Keychain

Providers are easy to write. Given the path to a secret, they either return its value or an error.

This is their contract:

* They take one argument, the path to where a secret is stored (a string).
* If retrieval is successful, they return the value on stdout with exit code 0.
* If an error occurs, they return an error message on stderr and a non-0 exit code.

The default path for providers is `/usr/libexec/cauldron/`. If one provider is in that path,
cauldron will use it. If multiple providers are in the path, you can specify which one to use
with the `--provider` flag or the environment variable `CAULDRON_PROVIDER`. If your providers are
placed outside the default path, give cauldron the full path to them.

[Open a Github issue](https://github.com/conjurinc/cauldron/issues) if you'd like to include your provider on this page.
