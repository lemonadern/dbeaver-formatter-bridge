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

## 選択中の SQL タブの統合テスト

Windows CI では、使い捨ての DBeaver にテスト用バンドルを追加し、実際の Workbench に SQL タブを開きます。
テスト用バンドルは配布 ZIP に含めません。
既存の JDK と DBeaver のライブラリでビルドでき、DB ドライバの追加は不要です。

```sh
go run ./scripts/ci/build-active-editor-fixture.go \
  -dbeaver-home "/path/to/dbeaver" -java-home "$JAVA_HOME"
```

テストでは `SQLEditor` のテスト用サブクラスと接続のテストダブルを使います。
接続設定の異なるタブの切り替え、対象がない場合のエラー、要求の検証、UI 待機のタイムアウトと復帰、CLI 経由の整形を確認します。
整形結果は DBeaver の `SQLFormattingStrategy` の結果と比較します。
通常の SQL エディタの初期化や、JDBC で実 DB に接続するテストではありません。
起動と結果の判定は [Windows テストスクリプト](../scripts/ci/windows-dbeaver.ps1) を参照してください。

実際の接続で確認する場合は、接続別の整形設定が異なる二つの SQL タブを用意します。
同じ SQL に対して GUI の文書全体の整形、`--connection ID`、`--active-editor` の結果を比較します。
タブを切り替えてから外部エディタに戻り、次の要求で設定が切り替わることも確認してください。

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

## VS Code 拡張の統合テスト

依存の取得と VSIX の作成後、リポジトリ直下から実行します。
Go でそのマシン用の CLI をビルドするため、Go も PATH に必要です。

```sh
npm --prefix extensions/vscode run test:vsix
```

初回は最低対応版の VS Code をダウンロードします。
Linux の GUI がない環境では `xvfb-run -a npm --prefix extensions/vscode run test:vsix` とします。
既存の VS Code 本体を使う場合は `VSCODE_EXECUTABLE_PATH`、別の版を取得する場合は `VSCODE_TEST_VERSION` を指定します。
これらはテストランナーの環境変数であり、拡張の利用者向け設定ではありません。

テストは一時的なユーザーデータと拡張配置先に完成した VSIX をインストールし、その拡張を別のテスト用拡張から操作します。
実行中の拡張の配置先を確認し、開発ツリーの JavaScript を誤って試験しないようにします。
CLI は空白と日本語を含む一時 PATH に配置し、実行後にテスト用ファイルを削除します。
通常はローカルの代替 RPC サーバーを使い、実 DB 接続は行いません。

未保存 SQL、三つの接続モード、文字列の保持、空結果、失敗、処理中の変更、タイムアウト、未信頼ワークスペースを確認します。
整形結果は VS Code が最小限の編集に変換する場合があるため、編集適用後の全文で比較します。
VS Code の文書が CRLF の場合、DBeaver が LF を返しても適用後は CRLF になります。
CLI の出力そのものを変更しないことは、子プロセス制御の単体テストで確認します。

実 DBeaver を使う場合は、プラグイン導入済みの DBeaver を起動し、接続済みの SQL タブを選んだ状態で以下を指定します。

```sh
DBEAVER_BRIDGE_CLI=/absolute/path/dbeaver-format \
DBEAVER_BRIDGE_WORKSPACE=/absolute/path/workspace6 \
npm --prefix extensions/vscode run test:vsix
```

この場合は `activeEditor` と `global` を、CLI 単体の出力と比較します。
Windows CI は配布物に含まれないテスト用 SQL タブを使うため、通常の SQL エディタの初期化や実 DB の SQL 方言まで確認したことにはなりません。
普段使う VS Code へ勝手に拡張を導入することもありません。
