# Plugin build dependencies

The plugin compiles against an installed DBeaver's Java libraries.
The preparation tool copies those libraries into a local p2 repository, the dependency format used by Eclipse and DBeaver.
Use the tool versions in [mise.toml](../../mise.toml).
Set `JAVA_HOME` and `PATH` accordingly, or run the commands through `mise exec --`.

## Build the plugin

Run from the repository root:

```sh
go run ./scripts/prepare-target.go -dbeaver-home /path/to/dbeaver
go run ./scripts/build-plugin.go
```

Pass the directory containing DBeaver's `plugins/` and `features/` directories.
On macOS, the `.app` path is also accepted.
The preparation tool uses `java` from `PATH`; use `-java /path/to/java` to choose another executable.

The plugin installation ZIP is written to `plugin/repository/target/`.
It contains the bridge plugin, its installation metadata and licenses.
Use this ZIP for distribution; the local dependency repository contains DBeaver's libraries.

## Refresh or remove dependencies

Run preparation again after changing the DBeaver installation.
Maven `clean` preserves the prepared dependencies.
Delete `plugin/target-platform/target/` to remove them and the temporary publisher configuration.

The [build helper](../../scripts/build-plugin.go) runs Maven `clean verify` and passes the local repository as an encoded file URI.
It also gives the plugin and feature a UTC timestamp qualifier, which distinguishes later snapshot builds when installing an update.
Builds started within the same second share a qualifier.

To invoke Maven directly, supply `-Ddbeaver.target.uri=<absolute file URI>` and `-DforceContextQualifier=<UTC timestamp>` as the helper does.

## Test path handling

```sh
go test ./scripts/build-plugin.go ./scripts/build-plugin_test.go
go test ./scripts/prepare-target.go ./scripts/build-plugin_test.go
```
