# github.com/cyberark/summon/provider

Functions to resolve and call a Summon provider.

`func Resolve(providerArg string) (string, error)`

Searches for a provider in this order:

1. `providerArg`, passed in via CLI
2. environment variable `SUMMON_PROVIDER`
3. check for directory `/usr/local/lib/summon`
   (or `%ProgramW6432%\Cyberark Conjur\Summon\Providers` on Windows):
   if it exist, search providers there
4. if all of the above do not exist: use 
   `<path_to_summon_excutable>\Providers` for searching providers (aka 'portable mode')

*Attention*: the provider search is limited to the first directory found
according to the priority list above. That means, if the system directory
exist the local directory will never be searched, even if the system directory
is empty. 

In order to migrate from system directory configuration to a local provider directory you need to move all providers to the local provider dir *AND* delete
the system directory.

`func Call(provider, specPath string) (string, error)`

Given a provider and secret's namespace, runs the provider to resolve
the secret's value.

`func CallInteractiveMode(provider string, secrets secretsyml.SecretsMap) (chan Result, chan error, func())`

Given a provider and secrets, runs the provider in interactive mode to resolve multiple
secret's values in a single process.