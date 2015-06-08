# github.com/conjurinc/summon/provider

Functions to resolve and call a Summon provider.

`func ResolveProvider(providerArg string) (string, error)`

Searches for a provider in this order:

1. `providerArg`, passed in via CLI
2. Environment variable `SUMMON_PROVIDER`
3. Executable in `/usr/libexec/summon/`.

`func CallProvider(provider, specPath string) (string, error)`

Given a provider and secret's namespace, runs the provider to resolve
the secret's value.
