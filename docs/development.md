# リポジトリ構成

CLI はインスタンスの特定、認証、HTTP 通信、標準入出力を担当します。
DBeaver プラグインは、接続先の方言と設定を選び、DBeaver のフォーマッタを呼び出します。
両者のデータ形式は [通信仕様](protocol.md) で定義します。

| ディレクトリ | 役割 |
|---|---|
| `cli/cmd/dbeaver-format/` | CLI のエントリーポイント |
| `cli/internal/instance/` | ワークスペースからインスタンス情報を読む |
| `cli/internal/client/` | DBeaver のローカル API を呼び出す |
| `cli/internal/command/` | 引数と標準入出力を扱う |
| `plugin/bundles/formatter-bridge/` | DBeaver に整形コマンドを追加するプラグイン |
| `plugin/features/` | DBeaver に表示するインストール単位の定義 |
| `plugin/repository/` | プラグイン導入用 ZIP の生成 |
| `plugin/target-platform/` | ビルド時に参照する DBeaver と Eclipse のライブラリ |
| `scripts/` | ビルド、配布、統合テストのコマンド |
| `.github/workflows/` | CI の定義 |
| `examples/efm-langserver/` | efm-langserver の設定例 |
| `tests/fixtures/` | 検証用 SQL |
| `licenses/` | CLI に含まれる第三者ソフトウェアのライセンス表示 |
| `extensions/vscode/` | PATH 上の CLI を呼ぶ VS Code 拡張と VSIX のビルド、導入テスト |

ビルドの実行手順は [README](../README.md#開発)、ライブラリの準備方法は [プラグインのビルド依存](../plugin/target-platform/README.md) を参照してください。

## 生成物

| 出力先 | 内容 |
|---|---|
| `build/` | OS と CPU ごとの CLI 実行ファイル |
| `dist/` | CLI 配布 ZIP、VSIX、それぞれのチェックサム |
| `plugin/repository/target/` | DBeaver に導入するプラグイン ZIP |
| `plugin/target-platform/target/` | ビルド用ライブラリと一時設定 |

ビルド用ライブラリには DBeaver 本体の JAR を含むため、プラグインの配布には `plugin/repository/target/` の ZIP を使います。

## VS Code 拡張の開発

Node.js と npm を使います。
開発用 Node.js は `mise.toml`、npm の開発依存は `extensions/vscode/package-lock.json` で固定します。
利用者には Node.js、Go、JDK の追加導入は不要ですが、CLI と DBeaver プラグインは別途必要です。

リポジトリ直下から実行します。

```sh
npm --prefix extensions/vscode ci --ignore-scripts
npm --prefix extensions/vscode test
npm --prefix extensions/vscode run package
go run ./scripts/ci/verify-vsix.go -write-checksum
```

VSIX は `dist/` に出力します。
パッケージングはルートの LICENSE と COPYRIGHT を拡張へコピーし、拡張の JavaScript と導入説明だけを収録します。
開発依存は VSIX に含めません。
`vsce` の認証用の間接依存には、既知の advisory を避けるため `@azure/identity` と `uuid` の override を設定しています。
これらは VSIX 作成時の開発依存であり、Marketplace の認証や公開は今回使用しません。
更新時は `npm audit` と VSIX 作成、収録物の検証を再実行してください。

実 VS Code を使う検証は [テストの実行](testing.md#vs-code-拡張の統合テスト) を参照してください。
