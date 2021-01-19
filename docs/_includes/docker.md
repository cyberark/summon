# Summon + Docker

Docker works best when you follow the 12-factor suggestion of storing your
config [in the environment](http://12factor.net/config). Your images are then
immutable artifacts that you can move through environments. This works great for
public config (DNS names, database ports, log levels, etc), but what
about sensitive config (database passwords, API tokens, etc)? You can bake
credentials into your images, but now you have to lock down your images and
rotation is difficult. You can repurpose tools like [consul](https://www.consul.io/)
or [etcd](https://coreos.com/etcd/) to serve secrets, but they are not meant for
passing around secrets.

Using Summon with Docker means that the secrets your application needs are declared
in `secrets.yml` and can be checked into source control right next to your Dockerfile.
Since Summon has pluggable providers, you aren't locked into any one solution for
managing your secrets.

Summon makes it easy to inject secrets as environment variables into your Docker
containers by taking advantage of Docker's CLI arguments (`--env-file` or, `--env` and
`--volume`. There are two options available. It's possible to mix and match as you see fit.

## Docker --env and --volume arguments

This is done on-demand by using the variable `@SUMMONDOCKERARGS` in the arguments of the
 process you are running with Summon. This variable is replaced by combinations of the
 Docker arguments `--env` and `--volume` such that the secrets injected by summon are
 passed into the Docker container. The `--volume` arguments allow memory-mapped temporary
 files from variables with the `!file` tag to be resolvable inside the container.

**NOTE:** Using the `!file` tag with `@SUMMONDOCKERARGS` assumes that the Docker CLI is
being run on the host that is used to create volume mounts to the container. For when
this is not the case simply avoid using the `!file` tag, but be mindful that in that case
you lose the benefits of memory-mapped temporary files.

```bash
$ summon -p keyring.py -D env=dev docker run @SUMMONDOCKERARGS deployer
Checking credentials
Deploying application
```

### @SUMMONDOCKERARGS Example

The example below demonstrates the use of @SUMMONDOCKERARGS. For the sake of brevity
we use an inline `secrets.yml` and the `/bin/echo` provider. Some points to note:

1. `summon` is invoking `docker` as the child process.
2. `@SUMMONDOCKERARGS` is replaced with a combination of `--env` and `--volume` arguments.
3. Variable `D` uses the `!file` tag and therefore is the only one that
results in a `--volume` argument. The path to this variable inside the container
is as it is on the host.

```bash
secretsyml='
A: |-
  A_value with
  multiple lines
B: B_value
C: !var C_value
D: !var:file D_value
'

# The substitution of @SUMMONDOCKERARGS the docker run command below results in
# something of the form:
#
# docker run --rm \
#  --env A --env B --env C --env D \
#  --volume /path/to/D:/path/to/D
#  alpine ...
#
# The output from the command is shown below the command.

summon --provider /bin/echo --yaml "${secretsyml}" \
 docker run --rm @SUMMONDOCKERARGS alpine sh -c '
printenv A;
printenv B;
printenv C;
cat $(printenv D);
'
# A_value with
# multiple lines
# B_value
# C_value
# D_value
```
## Docker --env-file argument
This is done on-demand by using the variable `@SUMMONENVFILE` in the arguments of the process
you are running with Summon. This variable points to a memory-mapped file containing
the variables and values from secrets.yml in VAR=VAL format.

```sh
$ summon -p keyring.py -D env=dev docker run --env-file @SUMMONENVFILE deployer
Checking credentials
Deploying application
```

### @SUMMONENVFILE Example

Let's say we have a deploy script that needs to access our application servers on
AWS and pull the latest version of our code. It should record the outcome of the
deployment to a datastore. To deploy, then, we need AWS keys and a MongoDB password.

### 1. Clone the Summon repository

This example is stored in the Summon repository in the `examples` folder. To get
started, we'll need to clone the Summon repository and navigate to the example folder.

```sh
$ git clone https://github.com/cyberark/summon.git
$ cd summon/examples/docker/
```

There are 3 key files in this directory.

**secrets.yml**

This is the file that Summon will read, and it contains a mapping of environment
variables to the name of secrets we want to fetch. Secrets *are* dependencies so
we should be able to track them in source control. `$env` is a variable that we
will supply at runtime with Summon's `-D` flag. This means that we can use one
`secrets.yml` file for all environments, swapping out `$env` as needed.

<script src="http://gist-it.appspot.com/github/cyberark/summon/blob/master/examples/docker/secrets.yml"></script>

**deploy.py**

A stubbed-out deploy script. It checks that you have the proper credentials
before attempting a deploy.

<script src="http://gist-it.appspot.com/github/cyberark/summon/blob/master/examples/docker/deploy.py"></script>

**Dockerfile**

Inherits from the [offical Python Docker image](https://registry.hub.docker.com/_/python/)
and runs the deploy script.

<script src="http://gist-it.appspot.com/github/cyberark/summon/blob/master/examples/docker/Dockerfile"></script>

### 2. Build and run the container

**Note:** [Install Docker](https://docs.docker.com/get-docker/) if you don't already
have it on your system.

Now we can build a Docker image and run our deploy script inside it.

```sh
$ docker build -t deployer .
...
$ docker run deployer
Checking credentials
AWS_ACCESS_KEY_ID not available!
AWS_SECRET_ACCESS_KEY not available!
MONGODB_PASSWORD not available!
```

Our deploy script is checking for available credentials. None are available, so it
doesn't run the deploy.

### 3. Install Summon and the keyring provider

Install Summon following the instructions in the [README](https://github.com/cyberark/summon/#install).

We'll use the [keyring provider](https://github.com/conjurinc/summon-keyring) for
this tutorial since it is cross-platform and doesn't require communication with a
secrets server. Install it following its [README instructions](https://github.com/cyberark/summon-keyring/#install).

### 4. Run the container with Summon

We want to provide our credentials to the container with Summon. Since we're using
the keychain provider, we'll put those secrets in our keychain. The keychain provider
is extensible and [supports many keyring implementations](https://pypi.org/project/keyring/)
out of the box. We'll use the OSX keychain for this example - modify the commands
depending on the keychain you use.

Remember the `$env` variable in our `secrets.yml`? We'll use 'dev' for this tutorial,
since we'd probably not use the keyring provider in production. Load the secrets into
your keychain. We'll use "summon" as the service name.

```sh
$ security add-generic-password -s "summon" -a "dev/aws_access_key_id" -w "AKIAIOSFODNN7EXAMPLE"
$ security add-generic-password -s "summon" -a "dev/aws_secret_access_key" -w "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
$ security add-generic-password -s "summon" -a "dev/mongodb_password" -w "blom0ey4hOj3We"
```

Now we can run Docker with Summon to provide our credentials.

```sh
$ summon -p keyring.py -D env=dev docker run --env-file @SUMMONENVFILE deployer
Checking credentials
Deploying application
```

Summon parsed `secrets.yml`, used the keychain provider to fetch values from our
keychain and made them available to Docker as `@SUMMONENVFILE`. Neat huh?

You can also view the value of `@SUMMONENVFILE` by simply `cat`ing it.

```sh
$ summon -p keyring.py -D env=dev cat @SUMMONENVFILE
AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
MONGODB_PASSWORD=blom0ey4hOj3We
AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

---

We hope Summon makes it easier for you to work with Docker and secrets. If you have
an idea for a new feature or notice a problem, please don't hesitate to
[open an issue or pull request on GitHub](https://github.com/cyberark/summon).
