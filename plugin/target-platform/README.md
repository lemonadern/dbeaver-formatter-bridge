# Installed DBeaver target platform

The build compiles against the bundles from a local DBeaver installation.
It does not build or download DBeaver itself.

From the repository root:

```sh
go run ./scripts/prepare-target.go -dbeaver-home /Applications/DBeaver.app
go run ./scripts/build-plugin.go
```

For Windows or Linux, pass the directory containing DBeaver's `plugins/` and `features/` directories.
The preparation tool uses `java` from `PATH`, or the executable supplied with `-java`.
The Java runtime must be new enough to run the installed DBeaver bundles.
The Maven build has been verified with JDK 25 and emits Java 21 bytecode.

The tool creates a private Equinox configuration and a local p2 repository under `plugin/target-platform/target/`.
It does not alter the DBeaver installation or its workspaces.
The local target contains copies of installed bundles and must not be distributed.
The distributable `plugin/repository/target/*.zip` contains only the bridge feature and plugin.

Re-run preparation after changing the DBeaver installation.
The build helper runs Maven `clean verify` and supplies the local target URI as an explicit property.
It encodes spaces and supports Windows drive letters and UNC paths.
Each invocation sets the snapshot bundle and feature qualifier to the current UTC timestamp (`yyyyMMddHHmmss`).
Rebuilding later produces a new installable version; snapshot builds are not byte-for-byte reproducible.
Builds started within the same second share a qualifier.
Run the tools within `mise exec -- ...` to use the pinned JDK and Maven, or set `JAVA_HOME` and `PATH` yourself.
Direct Maven invocations must supply `-Ddbeaver.target.uri=<absolute file URI>` and a fresh `-DforceContextQualifier=<UTC timestamp>`.
Maven clean keeps the prepared target.
Delete `plugin/target-platform/target/` to remove the local target and publisher configuration.

URI rules are checked on every host with:

```sh
go test ./scripts/build-plugin.go ./scripts/build-plugin_test.go
go test ./scripts/prepare-target.go ./scripts/build-plugin_test.go
```

Windows batch invocation and real Windows/Linux DBeaver execution require those operating systems; cross-compilation alone does not verify them.
