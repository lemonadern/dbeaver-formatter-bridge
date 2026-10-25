# CI と配布物

[GitHub Actions のワークフロー](../.github/workflows/ci.yml) は、push、pull request、手動実行でテストと配布物の作成を行います。
対象 OS とツールの版はワークフローを参照してください。

## テスト

CLI のジョブでは、各 OS で単体テスト、データ競合の検出、静的検査、ビルド用パスのテストを実行します。
Linux のジョブでは配布対象の CLI をまとめてビルドし、配布 ZIP の内容、ライセンス、チェックサム、改変した入力の拒否を検証します。

Windows のジョブでは、公式 DBeaver を一時ディレクトリへ展開し、プラグインのビルドと導入後に GUI を起動します。
その DBeaver に CLI から整形を依頼し、Unicode、LF/CRLF、空入力、失敗時の出力、並行要求を検証します。
PowerShell と Git Bash からの UTF-8 入力、空白と日本語を含む CLI とワークスペースのパスも対象です。
取得する DBeaver とチェックサム、起動手順は [Windows テストのスクリプト](../scripts/ci/windows-dbeaver.ps1) を参照してください。

## 配布物を取得する

GitHub の **Actions → CI** で成功した実行を開き、**Artifacts** からダウンロードします。

| 名前 | 内容 |
| --- | --- |
| `cli-distributions` | OS と CPU ごとの CLI 配布 ZIP と `SHA256SUMS` |
| `plugin-update-site` | DBeaver に導入するプラグイン導入用 ZIP |

CLI の ZIP には実行ファイル、ライセンス文書、ビルド情報を含めます。
プラグインの ZIP には、プラグイン本体と導入用のメタデータを含めます。
導入方法は [導入手順](installation.md) を参照してください。

Windows のテストが失敗した場合は、`windows-dbeaver-failure-report` に失敗した段階と起動エラーの要約を保存します。
インスタンスの認証情報と DBeaver の生ログは保存対象から除外します。

## CLI の配布物を手元で作る

リポジトリ直下で実行します。

```sh
go run ./scripts/build-cli.go
go test -v ./scripts/package-cli.go ./scripts/package-cli_test.go
go run ./scripts/package-cli.go
```

出力先は `dist/` です。
Go や依存を更新する際は、[ライセンス一覧](../licenses/README.md) も更新してください。
パッケージングではバイナリのビルド情報とライセンス文書のハッシュを照合します。
