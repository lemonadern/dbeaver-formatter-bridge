# DBeaver Formatter Bridge

起動中の DBeaver に SQL の整形を依頼する CLI と DBeaver プラグインです。
DBeaver が選択しているフォーマッタを呼び出し、その接続の SQL 方言と整形設定を使用します。
エディタとの接続は efm-langserver に任せ、専用の VS Code 拡張は将来追加する構成です。

**開発版です。**
macOS arm64 / DBeaver CE 26.2.2 で GUI・CLI・efm の連携を検証しています。
Windows と Linux はビルド確認までで、実機での導入・GUI 連携は未検証です。
対応状況の詳細は [互換性](docs/compatibility.md) を参照してください。

## 利用

1. [導入手順](docs/installation.md) に従い、DBeaver にプラグインを導入して再起動します。
2. OS に合う CLI を `dbeaver-format`（Windows は `.exe`）という名前で PATH に配置します。
3. 起動した DBeaver のワークスペースを明示し、SQL を標準入力へ渡します。

```sh
dbeaver-format --workspace "/path/to/DBeaverData/workspace6" < input.sql
```

接続ごとの設定を使う場合は、接続済みの DBeaver 接続 ID を指定します。
接続名ではありません。

```sh
dbeaver-format --workspace "/path/to/workspace6" --connection "connection-id" < input.sql
```

接続を省略すると標準 SQL 方言とグローバル設定を使います。
アクティブなエディタの接続を推測しません。
インデントは DBeaver のテキストエディタ設定を使用し、必要なら `--indent "  "` で上書きできます。

- 入力は UTF-8 の SQL、最大 1 MiB。
- 標準出力は整形結果だけです。末尾改行を追加しません。
- 入力ファイルは上書きしません。
- 失敗時は標準エラー出力と非ゼロ終了コードを返し、整形結果を出力しません。
- 文書全体の整形に対応します。範囲整形には対応しません。
- タイムアウトは `--timeout 10s` などで指定します。既定は 10 秒です。

## エディタとの接続

[efm 設定例](examples/efm/README.md) を参照してください。
未保存のバッファを標準入力へ渡すため、SQL ファイルを先に保存する必要はありません。

## 開発

既存の Go 1.27.2、Maven 3.10.0、JDK 25 を使用します。
プラグインのコンパイル対象は Java 21、Tycho は 5.0.4 です。
`mise.toml` は採用版の宣言であり、ビルドスクリプトがツールをインストールすることはありません。
Maven の初回ビルドでは通常のビルド依存を取得します。

```sh
# リポジトリ直下で、既存 DBeaver からローカルのビルド依存を用意する
go run ./scripts/prepare-target.go -dbeaver-home /Applications/DBeaver.app -java "$JAVA_HOME/bin/java"
# ローカル p2 ターゲットの URI を安全に渡してビルドする
go run ./scripts/build-plugin.go
# Windows/macOS/Linux × amd64/arm64 を build/ へ出力する
go run ./scripts/build-cli.go
# CLI のテスト
cd cli
go test -race ./...
go vet ./...
```

生成される更新サイトは `plugin/repository/target/formatter-bridge-update-site-0.1.0-SNAPSHOT.zip` です。
ビルド用に取得した DBeaver の JAR は更新サイトに含めません。
[検証手順](docs/testing.md)、[通信仕様](docs/protocol.md)、[構成と今後の作業](docs/development.md) も参照してください。

## ライセンス

Apache-2.0。
DBeaver は別製品で、本リポジトリには DBeaver 本体・ドライバ・認証情報を含めません。
