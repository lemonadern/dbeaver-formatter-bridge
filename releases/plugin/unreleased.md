# Unreleased

## Summary

DBeaver の更新サイトからプラグインを導入、更新できる配布方式を追加します。

## Changes

- プラグイン専用のタグと Release を用意し、CLI と VS Code 拡張から独立して公開します。
- GitHub Pages の固定 URL を更新サイトの入口とし、各版のファイルは GitHub Releases から取得します。
- 従来の ZIP によるオフライン導入も維持します。

## Compatibility

既存の正式版 0.1.0 を導入済みの場合は、更新サイトの URL を追加して新版へ更新します。
DBeaver の対応版と CLI の対応範囲は、公開する版の検証結果に合わせて記載します。
