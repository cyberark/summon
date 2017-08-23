# unreleased

#v0.6.5
* Minor release, no functionality changes
  - Improved Jenkins CI pipeline.
  - Binaries are now built for more distributions (see `PLATFORMS` in [build.sh](build.sh)).
  - Simpler docker-compose development environment.

#v0.6.4
* Don't rely on executable bit on the provider; instead provide descriptive error if it fails to run - [Issue #40](https://github.com/cyberark/summon/issues/40)

#v0.6.3
* Summon now passes the child exit status to the caller - [PR #39](https://github.com/cyberark/summon/pull/39)

#v0.6.2
* Added 'default' section support, this is an alias for 'common' - [PR #37](https://github.com/cyberark/summon/pull/37)

#v0.6.1
* Support Boolean literals - [PR #35](https://github.com/cyberark/summon/pull/35)

# v0.6.0
* Write temporary files to home directory if possible

# v0.5.0
* added `-e`/`--environment` flag

# v0.4.0
* **breaking change** Default provider path is now `/usr/local/lib/summon`.

# v0.3.3
* Now fails more gracefully on unknown flags

# v0.3.2
* `@SUMMONENVFILE` is now ensured to contain a trailing newline [GH-22](https://github.com/cyberark/summon/issues/22)

# v0.3.1
* Integer values set in secrets.yml are now parsed correctly [GH-21](https://github.com/cyberark/summon/issues/21)

# v0.3.0
* Install bash completions if available
* Switch to tar.gz instead of .zip
* Try to find provider in the default path if just a name given
* Allow -p argument to override SUMMON_PROVIDER envar
* Check if provider exists and is executable

# v0.2.1
* Improve provider path handling [5df0fde](https://github.com/cyberark/summon/commit/5df0fdeb182884371ad647d0a9493a5e07d3e0e4)

# v0.2.0
* -D variable interpolation now plays nicely with the shell
* `@SUMMONENVFILE` for better Docker integration

# v0.1.2
* Fix --help and --version flags
* Vendor dependencies with Godep

# v0.1.1
* Attach stdin to allow running interactive processes wrapped with summon
* Changed name from 'cauldron' to 'summon'

# v0.1.0
* Initial release
