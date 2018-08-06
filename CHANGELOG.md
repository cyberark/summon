# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.6.7](https://github.com/cyberark/summon/releases/tag/v0.6.7) - 2018-08-06
### Added
- deb and rpm packages
### Changed
- Update build/package process to use [goreleaser](https://github.com/goreleaser/goreleaser).

## [v0.6.6](https://github.com/cyberark/summon/releases/tag/v0.6.6) - 2018-02-06
### Changed
- stdout is no longer buffered inside summon. This should greatly decrease the memory footprint of long-running processes wrapped by summon. Closes [#63](https://github.com/cyberark/summon/issues/63).

## [v0.6.5](https://github.com/cyberark/summon/releases/tag/v0.6.5) - 2017-08-23
### Changed
- Improved Jenkins CI pipeline.
- Binaries are now built for more distributions (see `PLATFORMS` in [build.sh](build.sh)).
- Simpler docker-compose development environment.

## [v0.6.4](https://github.com/cyberark/summon/releases/tag/v0.6.4) - 2017-04-12
### Changed
* Don't rely on executable bit on the provider; instead provide descriptive error if it fails to run - [Issue #40](https://github.com/cyberark/summon/issues/40)

## [v0.6.3](https://github.com/cyberark/summon/releases/tag/v0.6.3) - 2017-03-13
### Fixed
* Summon now passes the child exit status to the caller - [PR #39](https://github.com/cyberark/summon/pull/39)

## [v0.6.2](https://github.com/cyberark/summon/releases/tag/v0.6.2) - 2017-01-23
### Added
* Added 'default' section support, this is an alias for 'common' - [PR #37](https://github.com/cyberark/summon/pull/37)

## [v0.6.1](https://github.com/cyberark/summon/releases/tag/v0.6.1) - 2016-12-19
### Added
* Support Boolean literals - [PR #35](https://github.com/cyberark/summon/pull/35)

## [v0.6.0](https://github.com/cyberark/summon/releases/tag/v0.6.0) - 2016-06-20
### Changed
* Write temporary files to home directory if possible

## [v0.5.0](https://github.com/cyberark/summon/releases/tag/v0.5.0) - 2016-06-08
### Added
* added `-e`/`--environment` flag

## [v0.4.0](https://github.com/cyberark/summon/releases/tag/v0.4.0) - 2016-03-01
### Changed
* **breaking change** Default provider path is now `/usr/local/lib/summon`.

## [v0.3.3](https://github.com/cyberark/summon/releases/tag/v0.3.3) - 2016-02-29
### Fixed
* Now fails more gracefully on unknown flags

## [v0.3.2](https://github.com/cyberark/summon/releases/tag/v0.3.2) - 2016-02-10
### Fixed
* `@SUMMONENVFILE` is now ensured to contain a trailing newline [GH-22](https://github.com/cyberark/summon/issues/22)

## [v0.3.1](https://github.com/cyberark/summon/releases/tag/v0.3.1) - 2016-02-03
### Fixed
* Integer values set in secrets.yml are now parsed correctly [GH-21](https://github.com/cyberark/summon/issues/21)

## [v0.3.0](https://github.com/cyberark/summon/releases/tag/v0.3.0) - 2015-08-21
### Changed
* Install bash completions if available
* Switch to tar.gz instead of .zip
* Try to find provider in the default path if just a name given
* Allow -p argument to override SUMMON_PROVIDER envar
* Check if provider exists and is executable

## [v0.2.1](https://github.com/cyberark/summon/releases/tag/v0.2.1) - 2015-06-29
### Fixed
* Improve provider path handling [5df0fde](https://github.com/cyberark/summon/commit/5df0fdeb182884371ad647d0a9493a5e07d3e0e4)

## [v0.2.0](https://github.com/cyberark/summon/releases/tag/v0.2.0) - 2015-06-23
### Added
* `@SUMMONENVFILE` for better Docker integration
### Fixed
* -D variable interpolation now plays nicely with the shell

## [v0.1.2](https://github.com/cyberark/summon/releases/tag/v0.1.2) - 2015-06-10
### Fixed
* Fix --help and --version flags
### Changed
* Vendor dependencies with Godep

## [v0.1.1](https://github.com/cyberark/summon/releases/tag/v0.1.1) - 2015-06-09
### Changed
* Attach stdin to allow running interactive processes wrapped with summon
* Changed name from 'cauldron' to 'summon'

## [v0.1.0](https://github.com/cyberark/summon/releases/tag/v0.1.0) - 2015-06-03
### Added
* Initial release
