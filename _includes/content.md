# overview

summon is a command-line tool that reads a file in secrets.yml format
and injects secrets as environment variables into any process. Once the
process exits, the secrets are gone.

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
[Github README for the project](https://github.com/conjurinc/summon).

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

<h1 id="examples">examples</h1>

Summon is meant to work with your existing toolchains. If you can access environment variables, you can use Summon.

Here are some specific examples of how you can use summon with your current tools.

* [Docker](http://conjurinc.github.io/summon/docker.html)

Let us know what tools you would like us to cover next at [oss@conjur.net](mailto:oss@conjur.net).

<h1 id="providers">providers</h1>

* [AWS S3](https://github.com/conjurinc/summon-s3)
* [Conjur](https://github.com/conjurinc/summon-conjur)
* [Chef encrypted data bags](https://github.com/conjurinc/summon-chefapi)
* [keyring](https://github.com/conjurinc/summon-keyring)
* [Keepass kdbx database file](https://github.com/mskarbek/summon-keepass)
* [Gopass](https://github.com/justwatchcom/gopass/blob/master/docs/summon-provider.md)

Providers are easy to write. Given the identifier of a secret, they either return its value or an error.

This is their contract:

* They take one argument, the identifier of a secret (a string).
* If retrieval is successful, they return the value on stdout with exit code 0.
* If an error occurs, they return an error message on stderr and a non-0 exit code.

The default path for providers is `/usr/local/lib/summon/`. If one provider is in that path,
summon will use it. If multiple providers are in the path, you can specify which one to use
with the `--provider` flag or the environment variable `SUMMON_PROVIDER`. If your providers are
placed outside the default path, give summon the full path to them.

[Open a Github issue](https://github.com/conjurinc/summon/issues) if you'd like to include your provider on this page.
