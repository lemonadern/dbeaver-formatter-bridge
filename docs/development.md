# 構成と今後の作業

- `cli/`：Go 標準ライブラリだけで動く CLI。インスタンス情報、HTTP、標準入出力を担当。
- `plugin/bundles/formatter-bridge/`：DBeaver の追加コマンドとフォーマッタ呼び出し。
- `plugin/features/`、`plugin/repository/`：導入単位と p2 更新サイト。
- `plugin/target-platform/`：ローカルのビルド依存。生成物は Git の対象外。
- `scripts/`：インストール済み DBeaver のビルド依存準備、配布物ビルド、統合検証。
- `examples/efm/`：OS ごとの設定例。
- `tests/fixtures/`：検証用 SQL。

将来の VS Code 拡張は `extensions/vscode/` に追加します。
インスタンス探索・認証・通信は CLI に任せ、拡張へ重複実装しない方針です。
初期実装にはこのディレクトリも Node.js 依存も追加していません。

今後のリリース前作業：

1. 実際に使う DB・DBeaver の版・接続を確定し、接続別設定を GUI エディタと比較する。
2. Windows/Linux で p2 導入、CLI、efm、空白・日本語を含む配置パスを検証する。
3. p2 更新・削除と CLI 更新の確認を行う。
4. 初回 CI の実行結果を確認し、署名・公証とリリース手順を整える。

3 OS の CLI テスト、実 DBeaver を含む Windows CI、ライセンス付きアーカイブとチェックサムの生成は実装済みです。
CI の実行状況と対象は [ci.md](ci.md) を参照してください。

DBeaver 本体のフルビルドは不要です。
ビルド用のローカル p2 リポジトリと、利用者へ渡す更新サイトは別物です。
`plugin/target-platform/target/` を配布しないでください。
