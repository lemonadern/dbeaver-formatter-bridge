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
| `examples/efm/` | efm-langserver の設定例 |
| `tests/fixtures/` | 検証用 SQL |
| `licenses/` | CLI に含まれる第三者ソフトウェアのライセンス表示 |

ビルドの実行手順は [README](../README.md#開発)、ライブラリの準備方法は [プラグインのビルド依存](../plugin/target-platform/README.md) を参照してください。

## 生成物

| 出力先 | 内容 |
|---|---|
| `build/` | OS と CPU ごとの CLI 実行ファイル |
| `dist/` | ライセンス文書を含む CLI 配布 ZIP とチェックサム |
| `plugin/repository/target/` | DBeaver に導入するプラグイン ZIP |
| `plugin/target-platform/target/` | ビルド用ライブラリと一時設定 |

ビルド用ライブラリには DBeaver 本体の JAR を含むため、プラグインの配布には `plugin/repository/target/` の ZIP を使います。
