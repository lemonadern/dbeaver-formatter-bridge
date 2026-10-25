# CLI binary third-party materials

These files accompany the Go 1.27.2 CLI binaries, alongside the project's root `LICENSE` and `COPYRIGHT`.
The CLI does not bundle DBeaver, Java, efm-langserver, or a Go compiler.

The six default builds use `CGO_ENABLED=0`, no custom build tags, and the default Go experiment/FIPS configuration.
`go list -deps` was inspected for Windows, macOS, and Linux on amd64 and arm64.
The standard library dependency closure includes the vendored `golang.org/x/crypto`, `x/net`, `x/sys`, and `x/text` packages.
Their unmodified LICENSE and PATENTS files are included separately, as are Go's own LICENSE and PATENTS.
Homebrew places the Go root LICENSE one directory above GOROOT; the retained text is from that installed Go 1.27.2 distribution.

`go-source-NOTICES.txt` retains source comment notice blocks for the Go math library (Sun fdlibm and Stephen Moshier/Cephes), amd64 runtime memmove (Lucent/Vita Nuova), fiat-crypto generated arithmetic, and the public-domain AES implementation.
`go-fiat-crypto-README.txt` retains the additional Fiat Cryptography attribution and license accompanying the generated nistec arithmetic.
These notices are included for the union of all six targets, including source routines that the linker may remove from a particular executable.

The dependency closure includes `crypto/internal/boring` Go wrappers/stubs and the Go FIPS implementation.
The default CGO-disabled builds do not select the BoringSSL `.syso` implementation, but the full upstream `go-boring-LICENSE.txt` is retained as additional provenance.
Fiat Cryptography notices used by Go FIPS arithmetic are retained regardless of whether FIPS mode is enabled.
This does not describe or certify the application as a validated FIPS module.

`manifest.json` records the source locations and SHA-256 hashes of the retained upstream texts.
The packaging script checks these hashes and rejects binaries with a different Go version, external modules, CGO enabled, custom tags/experiments, or non-default FIPS build settings.
Changing the Go version or build configuration requires reviewing this inventory again.
