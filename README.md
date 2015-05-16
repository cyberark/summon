# cauldron

Parse secrets.yml to export environment variables fetched from a trusted store

## Usage

TODO

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
