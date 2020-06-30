# github.com/cyberark/summon/provider

Functions to resolve and call a Summon provider.

`func Resolve(providerArg string) (string, error)`

Searches for a provider in this order:

1. `providerArg`, passed in via CLI
2. Environment variable `SUMMON_PROVIDER`
3. Executable in `/usr/local/lib/summon`
(or `%ProgramW6432%\Cyberark Conjur\Summon\Providers` and `<path_to_summon_excutable>\Providers`  on Windows).

`func Call(provider, specPath string) (string, error)`

Given a provider and secret's namespace, runs the provider to resolve
the secret's value.
