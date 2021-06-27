# github.com/cyberark/summon/provider

Functions to resolve and call a Summon provider.

`func Resolve(providerArg string) (string, error)`

Searches for a provider in this order:

1. `providerArg`, passed in via CLI
2. Environment variable `SUMMON_PROVIDER`
3. Executable in `/usr/local/lib/summon`
   (or `%ProgramW6432%\Cyberark Conjur\Summon\Providers` on Windows).
4. If all of the above do net exist: look for Executable in 
   `<path_to_summon_excutable>\Providers` (aka 'portable mode')

`func Call(provider, specPath string) (string, error)`

Given a provider and secret's namespace, runs the provider to resolve
the secret's value.
