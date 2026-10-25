# CLI third-party licenses

This directory contains the third-party license materials distributed with the CLI archives.
The project's own terms and copyright attribution are in the root `LICENSE` and `COPYRIGHT`.

## License inventory

| Files | Covered materials |
|---|---|
| `go-LICENSE.txt`, `go-PATENTS.txt` | Go runtime and standard library |
| `go-x-*-LICENSE.txt`, `go-x-*-PATENTS.txt` | Standard-library vendored `golang.org/x/crypto`, `x/net`, `x/sys`, and `x/text` |
| `go-source-NOTICES.txt` | Source notices for Sun fdlibm and Stephen Moshier/Cephes math routines, Lucent/Vita Nuova amd64 memmove, fiat-crypto arithmetic, and public-domain AES code |
| `go-fiat-crypto-README.txt` | Fiat Cryptography attribution and license for generated nistec arithmetic |
| `go-boring-LICENSE.txt` | Upstream license accompanying Go's BoringSSL integration |

The inventory covers Windows, macOS, and Linux on amd64 and arm64.
Archives include the combined notices for all six targets, including routines that the linker may remove from an individual executable.
Go's FIPS arithmetic uses fiat-crypto notices included here.
The default CGO-disabled builds select Go wrappers and stubs from `crypto/internal/boring`, but not the BoringSSL `.syso` implementation.
The BoringSSL license is retained as supplementary upstream material.

## Packaging checks

[`manifest.json`](manifest.json) records the Go version, source locations, and SHA-256 hashes of the retained upstream texts.
[`scripts/package-cli.go`](../scripts/package-cli.go) verifies those hashes and checks each binary's embedded build information before packaging.
Accepted binaries use the inventoried Go version, `CGO_ENABLED=0`, no external modules or custom build tags, and the default experiment and FIPS build settings.

Review this inventory when changing the Go version or build configuration.
The upstream license texts and their hashes must be updated together.
