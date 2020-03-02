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

### Releasing

The following checklist should be followed when creating a release:

- [ ] Open a PR with the following changes for `master` branch and wait for it to be merged:
  - [ ] Bump release version in [`pkg/summon/version.go`](pkg/summon/version.go).
  - [ ] Bump version in [`CHANGELOG.md`](CHANGELOG.md).
- [ ] Create a draft release:
  - [ ] Create an [annotated tag](https://git-scm.com/book/en/v2/Git-Basics-Tagging#_annotated_tags)
of the merged commit with the name `v<version>` and push it to the repository.
  - [ ] Using the GitHub UI, create a release using the tag created earlier as the starting point.
  - [ ] Name the release the same as the tag.
  - [ ] Include in the release notes all changes from CHANGELOG that are being released.
- [ ] Attach the relevant assets to the release:
  - [ ] [`Build`](./build) the release.
  - [ ] Attach `dist/summon-darwin-amd64.tar.gz` to release.
  - [ ] Attach `dist/summon-linux-amd64.tar.gz` to release.
  - [ ] Attach `dist/summon-windows-amd64.tar.gz` to release.
  - [ ] Attach `dist/summon.rpm` to release.
  - [ ] Attach `dist/summon.deb` to release.
  - [ ] Attach `dist/CHANGELOG.md` to release.
  - [ ] Edit `dist/SHA256SUMS.txt` to include only the attached files above.
  - [ ] Attach `dist/SHA256SUMS.txt` to the release.
- [ ] Publish the release as a "pre-released".
- [ ] TBD: Perform smoke tests of all attached files in release.
- [ ] Publish the release as a regular release.
- [ ] Update homebrew tools
  - [ ] In [`cyberark/homebrew-tools`](https://github.com/cyberark/homebrew-tools) repo, update
  the [`summon.rb` formula](https://github.com/cyberark/homebrew-tools/blob/master/summon.rb#L4-L6) with a PR
  using the file `dist/summon.rb`.
  - [ ] Once the PR is merged, verify that summon works by smoke testing it on OSX.
