# cauldron

Parse secrets.yml to export environment variables fetched from a trusted store

## Usage

*Very much WIP*

By default, cauldron will look for `secrets.yml` in the directory it is called from.

You can specify a location with the `-f` flag, like so:

```
cauldron -f=/etc/mysecrets.yml
```

View help and other flags with `cauldron -h`.

## Development

1. Install [gpm](https://github.com/pote/gpm), [gpm-local](https://github.com/technosophos/gpm-local) and [gvp](https://github.com/pote/gvp)
2. Set your local GOPATH and install dependencies: `source gvp in; gpm`
3. Alias the local project into .godeps: `gpm local name github.com/conjurinc/cauldron`

Run the project with `go run *.go`.

Run tests with `./test.sh`.

## Building

To build 64bit versions for Linux, OSX and Windows:

```
./build.sh
```

Binaries will be placed in `pkg/`.
