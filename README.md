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

To isolate pinned dependencies to this project:

1. Install [gpm](https://github.com/pote/gpm) and [gvp](https://github.com/pote/gvp).
2. Run `source gvp in; gpm` to set local GOPATH and install dependencies.

Run the project with `go run *.go`.

## Building

To build 64bit versions for Linux, OSX and Windows:

```
./build.sh
```

Binaries will be placed in `pkg/`.
