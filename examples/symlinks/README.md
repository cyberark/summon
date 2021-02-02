# Using Symbolic links to specify a fixed file name

The scripts here can be used with Summon to populate a specific file
as was noted in this [issue](https://github.com/cyberark/summon/issues/190).
Here are two examples of using Summon with symbolic links.

## Method 1

When you have a relatively small number of secrets variables that you'd
like to map to fixed file locations, you can invoke Summon with
commands to explicitly add symbolic links before, and to remove those
symbolic links after calling your application script.

__For example:__

Set the provider environmental variable and use `cat` as the user app.

```
summon sh -c 'ln -sf $FOO secrets-dir/foo.example.com &&
cat secrets-dir/foo.example.com && rm secrets-dir/foo.example.com'
```

## Method 2

When you have several secrets variables that you'd like to map to
fixed locations, then you may want to use the `summon-symlinks` helper
script that is in the example directory.

This script provides a wrapper for a Summon subcommand to
perform the following:

- Add symbolic links mapping tempfiles for given secrets variables to
   fixed locations (before invoking subcommand)
- Remove these symbolic links (after invoking subcommand)

Summon-symlinks uses an additional yaml file called secrets-symlinks.yml.
This yaml file maps the secrets variable to the fixed file location.
You can specify what file that `summon-symlinks` will use by setting
the `SUMMON_SYMLINKS` environmental variable.
If `SUMMON_SYMLINKS` is not set it defaults to secrets-symlinks.yml.

__For example:__

Set the provider environmental variable and use `cat` as the user app.

```
summon ./summon-symlinks cat secrets-dir/foo.example.com
```

__NOTE: Care should be take to avoid running the commands described
for either Method 1 or Method 2 concurrently.
In these examples we remove the symbolic link as the last step.
If multiple session are running concurrently the symbolic link could be
removed by the first session that ends. If removing the symbolic link is
ommitted, the symbolic link will be orphaned.__

The above examples were tested in a Linux (Ubuntu) end macOS nvironment.
