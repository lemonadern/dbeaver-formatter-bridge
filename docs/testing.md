# テストの実行

以下のコマンドは、リポジトリ直下で実行します。
必要な開発ツールは [README](../README.md#開発) を参照してください。

## CLI の単体テスト

```sh
go -C cli test -race -timeout 30s ./...
go -C cli vet ./...
```

インスタンス情報の読み取り、認証、HTTP 通信、タイムアウト、JSON、UTF-8、入力上限、失敗時の標準出力を検証します。
HTTP テストにはローカルのテスト用サーバーを使います。

## ビルド用パスのテスト

```sh
go test ./scripts/build-plugin.go ./scripts/build-plugin_test.go
go test ./scripts/prepare-target.go ./scripts/build-plugin_test.go
```

Windows のドライブと UNC パス、空白、日本語を含むパスの URI 変換を検証します。

## CLI と DBeaver の統合テスト

プラグインを導入した DBeaver を起動し、CLI をビルドしてから実行します。

```sh
go run ./scripts/test-bridge.go --cli /absolute/path/dbeaver-format --workspace /absolute/path/workspace6
```

文字列中の日本語、絵文字、引用符、バックスラッシュ、LF/CRLF、空入力、接続 ID 不明、タイムアウトを検証します。
異なるインデントを指定した並行要求について、単独実行の結果との一致も確認します。

手動で入力する SQL の例には [strings.sql](../tests/fixtures/strings.sql) を使えます。

## efm-langserver の統合テスト

この検証スクリプトは macOS と Linux のシェル向けです。
プラグインを導入した DBeaver と、CLI、efm-langserver を用意して実行します。

```sh
go run ./scripts/test-efm.go \
  --efm /absolute/path/efm-langserver \
  --cli /absolute/path/dbeaver-format \
  --workspace /absolute/path/workspace6
```

実際の efm-langserver へ LSP の初期化、文書を開く通知、整形要求を送り、未保存の SQL の整形結果を CLI の直接出力と比較します。
CLI が失敗した場合に、文書への編集が返らないことも確認します。

## 配布物のテスト

CLI 配布物の検証には、各 OS 向けのバイナリが必要です。

```sh
go run ./scripts/build-cli.go
go test -v ./scripts/package-cli.go ./scripts/package-cli_test.go
```

配布 ZIP の実行ファイル、ライセンス文書、実行権限、チェックサムを検証します。
ライセンス文書の改変や異なるビルド設定のバイナリを、パッケージングが拒否することも確認します。

プラグイン ZIP の検査は、プラグインをビルドした後に実行します。

```sh
go run ./scripts/ci/verify-site.go
```

導入用 ZIP にプラグインとインストール単位の JAR があり、両方に LICENSE と COPYRIGHT が含まれることを検証します。
自動実行するテストと成果物の取得方法は [CI](ci.md) を参照してください。
