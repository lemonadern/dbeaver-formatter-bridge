# CI と配布物

[GitHub Actions のワークフロー](../.github/workflows/ci.yml) は、`main` への push、pull request、手動実行でテストと配布物の作成を行います。
作業ブランチへの push だけでは実行せず、PR の作成・更新で検証します。
同じ PR が更新された場合は、その PR の古い実行を自動キャンセルします。
ビルドとテストの定義は [build-test.yml](../.github/workflows/build-test.yml) にまとめ、通常 CI とリリースで共有します。
対象 OS とツールの版はこの定義を参照してください。
最初に対象別 `VERSION` と生成済み定義の一致、リリース用ツールのテストを確認します。

## テスト

CLI のジョブでは、各 OS で単体テスト、データ競合の検出、静的検査、ビルド用パスのテストを実行します。
Linux のジョブでは配布対象の CLI をまとめてビルドし、配布 ZIP の内容、ライセンス、チェックサム、改変した入力の拒否を検証します。

Windows のジョブでは、公式 DBeaver を一時ディレクトリへ展開し、プラグインのビルドと導入後に GUI を起動します。
その DBeaver に CLI から整形を依頼し、Unicode、LF/CRLF、空入力、失敗時の出力、並行要求を検証します。
PowerShell と Git Bash からの UTF-8 入力、空白と日本語を含む CLI とワークスペースのパスも対象です。
テスト用 SQL タブによる `--active-editor` の検証範囲は [テストの実行](testing.md#選択中の-sql-タブの統合テスト) を参照してください。
取得する DBeaver とチェックサム、起動手順は [Windows テストのスクリプト](../scripts/ci/windows-dbeaver.ps1) を参照してください。

VSIX のジョブでは共通の VSIX を一度作成し、収録ファイルとライセンスを検証します。
同じ VSIX を Windows、macOS、Linux の隔離した VS Code に導入し、実 CLI と代替 RPC サーバーを使って三つの接続モードを検証します。
最低対応版の VS Code を使い、未信頼ワークスペースで拡張が無効になることも確認します。
Windows の実 DBeaver 試験では、既存の SQL タブの試験完了後にインストール済み拡張から `activeEditor` と `global` を検証します。
接続 ID モードの成功試験は代替 RPC によるもので、実 DB や JDBC 接続の検証とは区別します。

## 独立版と更新サイトの検証

`releases/compatibility.json` で公開済みの最小対応版とダウンロード URL、SHA-256 を固定しています。
Windows では新プラグインと旧 CLI、新 CLI と旧プラグイン、新 CLI と旧 VSIX、新 VSIX と旧 CLI の組合せを検証します。
p2 の試験では隔離した DBeaver のコピーを使い、実配布の旧版を導入し、一時 HTTP サーバーで提供する composite と平坦な p2 から試験用新版へ更新します。
本番 URL とローカル試験用 URL の差は別途検証し、本番 Release は作成しません。

## 配布物を取得する

GitHub の **Actions → CI** で成功した実行を開き、**Artifacts** からダウンロードします。

| 名前 | 内容 |
| --- | --- |
| `cli-distributions-<run ID>-<生成 attempt>` | OS と CPU ごとの CLI 配布 ZIP と `SHA256SUMS` |
| `plugin-update-site-<run ID>-<生成 attempt>` | DBeaver に導入するプラグイン導入用 ZIP |
| `vscode-vsix-<run ID>-<生成 attempt>` | ローカル導入用 VSIX と `VSIX-SHA256SUMS` |

artifact 名に生成 attempt を含め、取得には生成ジョブが返した artifact ID を使います。
失敗したジョブだけを再実行しても、先に成功した別 attempt のファイルを取り違えないためです。
末尾の集約ジョブは各対象の配布物をリモートへ公開せず検証し、`release-assets-<run ID>-<attempt>` に保存します。

CLI の ZIP には実行ファイル、ライセンス文書、ビルド情報を含めます。
プラグインの ZIP には、プラグイン本体と導入用のメタデータを含めます。
VSIX は[拡張の導入手順](../extensions/vscode/README.md)、CLI とプラグインは[導入手順](installation.md)を参照してください。
artifact の保存期間は 7 日です。
VSIX artifact はビルドのジョブで保存するため、利用時には後続の導入テストと実 DBeaver のジョブも成功していることを確認してください。

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

## リリースの発動条件

[Release](../.github/workflows/release.yml) は `plugin/vX.Y.Z`、`cli/vX.Y.Z`、`vscode/vX.Y.Z` タグの push を入口にします。
通常の PR と main の push では公開処理を実行しません。
対象のタグ、版定義、リリースノートと既存 Release を確認し、新規の場合だけ共通のビルドと試験を呼びます。
CLI の公開前には、一時キャッシュへの `go install` も確認します。
ビルドは読み取り権限、Release 公開は `contents: write` と来歴確認用の `actions: read` を使用します。
plugin の公開後に限り、Pages の composite 索引を生成してデプロイします。
Pages の権限はデプロイジョブだけに付け、公開 URL と参照先のハッシュまで確認します。

リリース同士は `queue: max` と `cancel-in-progress: false` で待機させ、通常 CI とは別グループで管理します。
PR ではリリース関連ファイルが変わる場合にだけ [待機確認](../.github/workflows/release-queue-probe.yml) の三ジョブも実行します。
本番タグの作成や Release 公開を行うテストではありません。
操作と復旧方針は[リリース手順](releases.md)を参照してください。
