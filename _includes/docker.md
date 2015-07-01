# Summon + Docker

Summon makes it easy to inject secrets as environment variables into your Docker containers by taking advantage of Docker's `--env-file` argument. This is done on-demand by using the variable `@SUMMONENVFILE` in the arguments of the process you are wrapping with Summon. This variable points to a memory-mapped file containing the variables and values from secrets.yml in VAR=VAL format.

```sh
$ summon docker run -it --env-file @SUMMONENVFILE bash
root@1878dc8c918d:/#
```

The major benefit of using Summon with Docker is the secrets your application
needs are declared in `secrets.yml` and can be checked into source control. Since Summon has pluggable providers, you can aren't locked into any one solution for managing your secrets.

## Example

Let's say we have a deploy script that needs to access our application servers on AWS and pull the latest version of our code. It should record the outcome of the deployment to a datastore.

### 1. Install Summon and the keyring provider

To install summon, either use the install script

```sh
$ curl -sSL https://raw.githubusercontent.com/conjurinc/summon/master/install.sh | bash
```

or [download the latest release](https://github.com/conjurinc/summon/releases/latest) and unzip it into your `PATH`.

We'll use the [keyring provider](https://github.com/conjurinc/summon-keyring) for this tutorial since it is cross-platform and doesn't require communication with a secrets server.

```sh
$ pip install keyring # dependency of the provider
$ mkdir -p /usr/libexec/summon
$ sudo curl -sSL -o /usr/libexec/summon/ring.py https://raw.githubusercontent.com/conjurinc/summon-keyring/master/ring.py
$ sudo chmod a+x /usr/libexec/summon/ring.py
```

### 2. Clone the example repository

The repository for this example is at [github.com/conjurdemos/summon-docker-tutorial](github.com/conjurdemos/summon-docker-tutorial). Clone that and we'll
get started.

```sh
$ git clone https://github.com/conjurdemos/summon-docker-tutorial.git
$ cd summon-docker-tutorial
```

There are 3 key files in this repository.

**deploy.py**

A stubbed-out deploy script. It checks that you have the proper credentials
before attempting a deploy. Secrets *are* dependencies so we should be able to
track them and verify their existence..

<script src="http://gist-it.appspot.com/github/conjurdemos/summon-docker-tutorial/blob/master/deploy.py"></script>

**Dockerfile**

Inherits from the [offical Python Docker image](https://registry.hub.docker.com/_/python/) and runs the deploy script. `requirements.txt` is empty, but if
it wasn't it would pip install your dependencies.

<script src="http://gist-it.appspot.com/github/conjurdemos/summon-docker-tutorial/blob/master/Dockerfile"></script>

**secrets.yml**

This is the file that Summon will read, a mapping of environment variables to
the name of secrets we want to fetch. Notice how the names of environment variables in `deploy.py` match the names in this file. `$env` is a variable
that we will supply at runtime.

<script src="http://gist-it.appspot.com/github/conjurdemos/summon-docker-tutorial/blob/master/secrets.yml"></script>

### 3. Build and run the container

*Note: [Install Docker](https://docs.docker.com/installation/) if you don't already have it on your system.*

Now we can build a Docker image and run our deploy script inside it.

```sh
$ docker build -t summon-docker .
...
$ docker run summon-docker
Checking credentials
AWS_ACCESS_KEY_ID not available!
AWS_SECRET_ACCESS_KEY not available!
MONGODB_PASSWORD not available!
```

Our deploy script is checking for available credentials. None are available, so it doesn't run the deploy.

### 4. Run the container with Summon

We want to provide our credentials to the container with Summon. Since we're using the keychain provider, let's put those secrets in our keychain. The keychain provider [supports many implementations](https://bitbucket.org/kang/python-keyring-lib/src/default/keyring/backends/). We'll use the OSX keychain for this example - modify the commands depending on the keychain you use.

Remember the `$env` variable in our `secrets.yml`? We'll use 'dev' for this tutorial, since we'd probably not use the keyring provider in production. Load the secrets into your keychain. We'll use "summon" as the service name.

```sh
$ security add-generic-password -s "summon" -a "dev/aws_access_key_id" -w "AKIAIOSFODNN7EXAMPLE"
$ security add-generic-password -s "summon" -a "dev/aws_secret_access_key" -w "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
$ security add-generic-password -s "summon" -a "dev/mongodb_password" -w "blom0ey4hOj3We"
```

Now we can wrap Docker with Summon to provide our credentials.

```sh
$ summon -p ring.py -D env=dev docker run --env-file @SUMMONENVFILE summon-docker
Checking credentials
Deploying application
```

Summon parsed `secrets.yml`, used the keychain provider to fetch values from our keychain and made them available to Docker as `@SUMMONENVFILE`. Neat huh?

You can view the value of `@SUMMONENVFILE` by `cat`ing it.

```sh
$ summon -p ring.py -D env=dev cat @SUMMONENVFILE
AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
MONGODB_PASSWORD=blom0ey4hOj3We
AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

---

We hope Summon makes it easier for you to work with Docker. If you have an idea for a new feature or notice a problem, please don't hesitate to [open an issue or pull request on GitHub](https://github.com/conjurinc/summon).