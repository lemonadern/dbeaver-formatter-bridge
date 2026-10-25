# 互換性

DBeaver との連携確認の対象版は **DBeaver CE 26.2.2** です。
下表は CLI の実行と DBeaver の整形処理への接続を分けて示します。
「ビルドのみ」は、対象バイナリを生成できることまでを表します。

| OS / CPU | CLI | DBeaver との連携 | efm-langserver との連携 |
|---|---|---|---|
| Windows amd64 | CI で実行 | CI で導入、GUI 起動、整形を確認 | 未確認 |
| Windows arm64 | ビルドのみ | 未確認 | 未確認 |
| macOS arm64 | 実行を確認 | GUI 起動と整形を確認 | 確認済み |
| macOS amd64 | ビルドのみ | 未確認 | 未確認 |
| Linux amd64 | CI で実行 | 未確認 | 未確認 |
| Linux arm64 | ビルドのみ | 未確認 | 未確認 |

Windows の CLI は PowerShell と Git Bash からの UTF-8 入力、空白と日本語を含むパスを検証対象に含みます。
CLI の実行に Go、Java、MinGW の追加インストールは不要です。
DBeaver 側は製品の Java を使用します。

## 整形結果の確認範囲

統合テストは標準 SQL 方言を使います。
実 DB 接続ごとの方言や整形設定と、DBeaver エディタの結果の比較は確認範囲に含んでいません。

改行は DBeaver のフォーマッタの出力に従います。
入力が CRLF でも、出力が LF になる場合があります。

## DBeaver の版との関係

連携には DBeaver の内部 API を使います。
DBeaver を更新する際は、プラグインのビルドと [統合テスト](testing.md#cli-と-dbeaver-の統合テスト) で動作を確認してください。
CI が使用する版は [Windows テストスクリプト](../scripts/ci/windows-dbeaver.ps1) に定義しています。
