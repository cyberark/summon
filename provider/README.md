# github.com/conjurinc/cauldron/provider

Functions to resolve and call a Cauldron provider.

`func ResolveProvider(providerArg string) (string, error)`

Searches for a provider in this order:
1. `providerArg`, passed in via CLI
2. Environment variable `CAULDRON_PROVIDER`
3. Executable in `/usr/libexec/cauldron/`.

`func CallProvider(provider, specPath string) (string, error)`

Given a provider and secret's namespace, runs the provider to resolve
the secret's value.