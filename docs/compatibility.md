# 互換性と検証状況

確認日：2026-10-25。
実 DBeaver を含む Windows CI と、3 OS の CLI テストが GitHub 上で成功しています。
[CI の内容と確認範囲](ci.md) を参照してください。
この一覧は実際に確認した範囲を表し、クロスビルドだけで実機対応済みとは扱いません。

| 対象 | ビルド | 実行・統合 |
|---|---|---|
| macOS arm64 | 成功 | DBeaver CE 26.2.2 GUI、CLI、efm 0.0.57 の連携を確認 |
| macOS amd64 | 成功 | 未確認 |
| Windows amd64 | 成功 | CI で CE 26.2.2 の p2 導入・GUI・CLI・PowerShell・Git Bash を確認 |
| Windows arm64 | 成功 | 未確認 |
| Linux amd64 / arm64 | 成功 | amd64 の CLI テストは CI で成功。実 DBeaver 連携と arm64 実行は未確認 |
| DBeaver CE 26.2.2 | Tycho ビルド成功 | macOS の一時構成で動作確認 |
| その他の DBeaver 版・商用版 | 未確認 | 未確認 |

ビルド時の依存はインストール済み CE 26.2.2（build 202610041844）から用意しました。
Maven 3.10.0、Tycho 5.0.4、Temurin 25.0.4.1、Go 1.27.2 を使用しています。
プラグインのバイトコードは Java 21 です。

macOS では一時 p2 環境で、正式な director 処理による導入、ZIP からの新版への更新、削除を確認しました。
既存 DBeaver の GUI メニューからの導入・更新・削除は未確認です。

確認済みの詳細なケースと再実行方法は [testing.md](testing.md) を参照してください。
対象 DB を指定した実接続の方言・接続別設定と GUI エディタの一致、p2 ZIP による通常インストール、更新・削除、通常の Windows 利用環境、Linux の実 DBeaver 連携、専用エディタの LSP クライアントは今後の検証対象です。
標準 SQL 方言での成功を各 DB 方言の対応完了とは扱いません。

改行は DBeaver のフォーマッタが返した結果をそのまま使います。
入力が CRLF でも同じ改行形式を維持する保証はなく、macOS の検証結果では LF になりました。
