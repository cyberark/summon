# Contributing to Summon

Thanks for your interest in Summon! Before contributing, please
take a moment to read and sign our <a href="https://github.com/cyberark/summon/blob/master/Contributing_OSS/CyberArk_Open_Source_Contributor_Agreement.pdf" download="summon_contributor_agreement">Contributor Agreement</a>.
This provides patent protection for all Summon users and allows CyberArk
to enforce its license terms. Please email a signed copy to
<a href="oss@cyberark.com">oss@cyberark.com</a>.

## Development

Run the project with:

```
go run cmd/main.go
```

### Testing

Tests are written using [GoConvey](http://goconvey.co/).
Run tests with `go test -v ./...` or `./test` (for CI).

### Building and packaging

To build versions for Linux, OSX and Windows:

```
./build
```

Binaries will be placed in `output/`.
