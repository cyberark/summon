# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## Unreleased

## [0.8.2] - 2020-06-23
### Added
- Summon now supports a `--version-providers` flag to display the versions of installed providers.
  [cyberark/summon#138](https://github.com/cyberark/summon/issues/138)
- Summon supports a `--up` flag that searches for secrets.yml going up, starting from the
  current working directory. This allows the secrets.yml file to be at any directory depth in a
  project, and it is no longer required to be in the current working directory if not specified
  with the `-f` flag.
  [#122](https://github.com/cyberark/summon/issues/122)

## [0.8.1] - 2020-03-02
### Changed
- Added ability to support empty variables [#124](https://github.com/cyberark/summon/issues/124)

### Added
- Added better errors for unknown tags found in the yaml
- Added ability to set a default variable value with `default='<value>'` tag [#38](https://github.com/cyberark/summon/issues/38)

## [0.8.0] - 2019-09-20
### Changed
- To ensure cleanup of files on non-windows platforms we now remain resident
  until the child is killed or it exits [#106](https://github.com/cyberark/summon/issues/106)
- Updated base Golang version to 1.13
- Made Linux builds create static binaries [#65](https://github.com/cyberark/summon/issues/65)

### Added
- Added gitleaks configuration

### Fixed
- Fixed broken website links

## [0.7.0] - 2019-07-11
### Changed
- Updated yaml.v1 dependency to [yaml.v3](gopkg.in/yaml.v3) in part to address
  [cyberark/secretless-broker#785](https://github.com/cyberark/secretless-broker/issues/785)
- Updates goreleaser config to address deprecated sections

### Fixed
- Bumps `ffi` in the `acceptance/` directory to address [this CVE](https://nvd.nist.gov/vuln/detail/CVE-2018-1000201)

### Added
- Added CONTRIBUTING.md for contribution guidelines for the project, including
  contributor agreement

## [0.6.11] - 2019-01-09
### Changed
- Added exporting of `SUMMON_ENV` if `-e` flag is present. Closes[#92](https://github.com/cyberark/summon/issues/92).

## [0.6.10] - 2019-01-03
### Fixed
- Windows subprocess loading is again run with exec.Command. Closes[#88](https://github.com/cyberark/summon/issues/88).

### Changed
- Windows detection of 'Program Files' folder improved.

## [0.6.9] - 2018-12-07
### Changed
- Updated codebase to use [go v1.11 modules](https://github.com/golang/go/wiki/Modules).
- Updated acceptance tests to use an automated test image builds and no makefiles.
- Made subprocess loading take place through execve. Fixes [#83](https://github.com/cyberark/summon/issues/83).

## [0.6.8] - 2018-09-14
### Added
- `-I/--ignore-all` [flag](https://github.com/cyberark/summon#flags), [PR #78](https://github.com/cyberark/summon/pull/78)

## [0.6.7] - 2018-08-06
### Added
- deb and rpm packages

### Changed
- Update build/package process to use [goreleaser](https://github.com/goreleaser/goreleaser).

## [0.6.6] - 2018-02-06
### Changed
- stdout is no longer buffered inside summon. This should greatly decrease the memory footprint of long-running processes wrapped by summon. Closes [#63](https://github.com/cyberark/summon/issues/63).

## [0.6.5] - 2017-08-23
### Changed
- Improved Jenkins CI pipeline.
- Binaries are now built for more distributions (see `PLATFORMS` in [build.sh](build.sh)).
- Simpler docker-compose development environment.

## [0.6.4] - 2017-04-12
### Changed
- Don't rely on executable bit on the provider; instead provide descriptive error if it fails to run - [Issue #40](https://github.com/cyberark/summon/issues/40)

## [0.6.3] - 2017-03-13
### Fixed
- Summon now passes the child exit status to the caller - [PR #39](https://github.com/cyberark/summon/pull/39)

## [0.6.2] - 2017-01-23
### Added
- Added 'default' section support, this is an alias for 'common' - [PR #37](https://github.com/cyberark/summon/pull/37)

## [0.6.1] - 2016-12-19
### Added
- Support Boolean literals - [PR #35](https://github.com/cyberark/summon/pull/35)

## [0.6.0] - 2016-06-20
### Changed
- Write temporary files to home directory if possible

## [0.5.0] - 2016-06-08
### Added
- added `-e`/`--environment` flag

## [0.4.0] - 2016-03-01
### Changed
- **breaking change** Default provider path is now `/usr/local/lib/summon`.

## [0.3.3] - 2016-02-29
### Fixed
- Now fails more gracefully on unknown flags

## [0.3.2] - 2016-02-10
### Fixed
- `@SUMMONENVFILE` is now ensured to contain a trailing newline [GH-22](https://github.com/cyberark/summon/issues/22)

## [0.3.1] - 2016-02-03
### Fixed
- Integer values set in secrets.yml are now parsed correctly [GH-21](https://github.com/cyberark/summon/issues/21)

## [0.3.0] - 2015-08-21
### Changed
- Install bash completions if available
- Switch to tar.gz instead of .zip
- Try to find provider in the default path if just a name given
- Allow -p argument to override SUMMON_PROVIDER envar
- Check if provider exists and is executable

## [0.2.1] - 2015-06-29
### Fixed
- Improve provider path handling [5df0fde](https://github.com/cyberark/summon/commit/5df0fdeb182884371ad647d0a9493a5e07d3e0e4)

## [0.2.0] - 2015-06-23
### Added
- `@SUMMONENVFILE` for better Docker integration

### Fixed
- -D variable interpolation now plays nicely with the shell

## [0.1.2] - 2015-06-10
### Fixed
- Fix --help and --version flags

### Changed
- Vendor dependencies with Godep

## [0.1.1] - 2015-06-09
### Changed
- Attach stdin to allow running interactive processes wrapped with summon
- Changed name from 'cauldron' to 'summon'

## [0.1.0] - 2015-06-03
### Added
- Initial release

[Unreleased]: https://github.com/cyberark/summon/compare/v0.8.2...HEAD
[0.8.1]: https://github.com/cyberark/summon/compare/v0.8.1...v0.8.2
[0.8.1]: https://github.com/cyberark/summon/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/cyberark/summon/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/cyberark/summon/compare/v0.6.11...v0.7.0
[0.6.11]: https://github.com/cyberark/summon/compare/v0.6.10...v0.6.11
[0.6.10]: https://github.com/cyberark/summon/compare/v0.6.9...v0.6.10
[0.6.9]: https://github.com/cyberark/summon/compare/v0.6.8...v0.6.9
[0.6.8]: https://github.com/cyberark/summon/compare/v0.6.7...v0.6.8
[0.6.7]: https://github.com/cyberark/summon/compare/v0.6.6...v0.6.7
[0.6.6]: https://github.com/cyberark/summon/compare/v0.6.5...v0.6.6
[0.6.5]: https://github.com/cyberark/summon/compare/v0.6.4...v0.6.5
[0.6.4]: https://github.com/cyberark/summon/compare/v0.6.3...v0.6.4
[0.6.3]: https://github.com/cyberark/summon/compare/v0.6.2...v0.6.3
[0.6.2]: https://github.com/cyberark/summon/compare/v0.6.1...v0.6.2
[0.6.1]: https://github.com/cyberark/summon/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/cyberark/summon/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/cyberark/summon/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/cyberark/summon/compare/v0.3.0...v0.4.0
[0.3.3]: https://github.com/cyberark/summon/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/cyberark/summon/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/cyberark/summon/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/cyberark/summon/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/cyberark/summon/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/cyberark/summon/compare/v0.1.2...v0.2.0
[0.1.2]: https://github.com/cyberark/summon/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/cyberark/summon/compare/v0.1.0...v0.1.1
[0.0.1]: https://github.com/cyberark/summon/releases/tag/v0.0.1
