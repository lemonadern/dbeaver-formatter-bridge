# DBeaver Formatter Bridge

DBeaver の SQL フォーマッタをコマンドラインから利用するためのツールです。
CLI が標準入力の SQL を起動中の DBeaver プラグインへ渡し、整形結果を標準出力に返します。
DBeaver の整形設定と、接続先の SQL 方言を使えます。

## 利用

1. [導入手順](docs/installation.md) に従い、DBeaver にプラグインを導入して再起動します。
2. OS に合う CLI を `dbeaver-format`（Windows は `.exe`）という名前で PATH に配置します。
3. DBeaver を起動した状態で、ワークスペースを指定して SQL を標準入力へ渡します。

```sh
dbeaver-format --workspace "/path/to/DBeaverData/workspace6" < input.sql
```

接続先の SQL 方言と整形設定を使う場合は、DBeaver で接続し、その接続 ID を指定します。

```sh
dbeaver-format --workspace "/path/to/workspace6" --connection "connection-id" < input.sql
```

DBeaver で選択中の SQL タブの接続を使う場合は、`--active-editor` を指定します。
DBeaver で対象の SQL タブを選び、その接続を接続済みにしてから実行します。

```sh
dbeaver-format --workspace "/path/to/workspace6" --active-editor < input.sql
```

`--connection` と `--active-editor` は同時に指定できません。
どちらも省略すると、標準 SQL 方言とグローバル設定を使います。
インデントは DBeaver のテキストエディタ設定に従い、`--indent "  "` で上書きできます。

- 入力は UTF-8 の SQL で、最大 1 MiB です。
- 文書全体を整形します。
- 標準出力は整形結果だけで、末尾改行を追加しません。
- エラーは標準エラー出力に書き出し、非ゼロの終了コードを返します。
- タイムアウトは `--timeout 10s` などで指定します。既定は 10 秒です。

対応する環境は [互換性](docs/compatibility.md) を参照してください。

## エディタとの接続

[efm-langserver の設定例](examples/efm-langserver/README.md) を参照してください。
未保存のバッファを標準入力へ渡せるため、SQL ファイルを先に保存する必要はありません。

## 開発

ビルドには Go、JDK、Maven と DBeaver が必要です。
ツールのバージョンは [mise.toml](mise.toml) を参照してください。
`JAVA_HOME` を JDK のインストール先に設定し、リポジトリ直下で実行します。
`-dbeaver-home` には DBeaver のインストール先を指定します（macOS では `/Applications/DBeaver.app` など）。

```sh
# ビルドで参照する DBeaver のライブラリを準備する
go run ./scripts/prepare-target.go -dbeaver-home "/path/to/dbeaver" -java "$JAVA_HOME/bin/java"
# プラグインをビルドする
go run ./scripts/build-plugin.go
# Windows、macOS、Linux の amd64 と arm64 向けに CLI をビルドする
go run ./scripts/build-cli.go
# CLI の配布 ZIP を作る
go run ./scripts/package-cli.go
# CLI のテストを実行する
cd cli
go test -race ./...
go vet ./...
```

DBeaver に導入するプラグイン ZIP は `plugin/repository/target/formatter-bridge-update-site-0.1.0-SNAPSHOT.zip` に生成されます。
CLI の OS 別配布 ZIP とチェックサムは `dist/` に生成されます。

[検証手順](docs/testing.md)、[通信仕様](docs/protocol.md)、[リポジトリ構成](docs/development.md) を参照してください。

## ライセンス

[Apache-2.0](LICENSE)
