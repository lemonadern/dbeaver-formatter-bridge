# Unreleased

## Summary

CLI の版と配布物を、プラグインと VS Code 拡張から独立して管理します。

## Changes

- CLI 専用の Release で Windows、macOS、Linux の配布 ZIP とチェックサムを公開します。
- 配布 ZIP のファイル名にバージョンを含めます。
- `--version` で CLI の版を確認でき、`go install` でもその版を表示します。

## Compatibility

`go install github.com/lemonadern/dbeaver-formatter-bridge/cli/cmd/dbeaver-format@<version>` を引き続き使用できます。
プラグインと VS Code 拡張の対応範囲は、公開する版の検証結果に合わせて記載します。
