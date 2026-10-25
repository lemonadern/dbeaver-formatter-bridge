# CI と配布物

GitHub Actions の定義は `.github/workflows/ci.yml` です。
`push`、`pull_request`、手動の `workflow_dispatch` で実行します。
リポジトリへ push する処理や、リリースを公開する処理はありません。

## CLI ジョブ

Ubuntu・macOS・Windows で CLI の単体テスト、race 検出、vet と URI テストを実行します。
Ubuntu では 6 種類の CLI をクロスビルドし、配布 ZIP の内容・第三者ライセンス・SHA-256 のテストも実行します。
成功時は `dist/*.zip` と `dist/SHA256SUMS` を artifact として保存します。
各 ZIP には実行ファイル、LICENSE、COPYRIGHT、Go 側のライセンス表示、BUILD-INFO.txt を含めます。

## 実 DBeaver の Windows ジョブ

`scripts/ci/windows-dbeaver.ps1` が次の処理を行います。

1. 公式の DBeaver CE 26.2.2 Windows x64 ZIP を取得し、固定した SHA-256 と照合する。
2. 一時ディレクトリへ展開し、その DBeaver を対象にプラグインをビルドする。
3. 更新サイトの bundle と feature に LICENSE と COPYRIGHT があることを検査する。
4. 展開した DBeaver に p2 director でプラグインを導入する。
5. 分離したユーザーディレクトリとワークスペースで DBeaver の GUI を起動する。
6. CLI に実際の整形要求を送り、起動完了を待つ。
7. Unicode・LF/CRLF・空入力・失敗時の出力・並行要求を検証する。
8. PowerShell と Git Bash から UTF-8 の SQL を渡し、文字列が保たれることを確認する。
9. 成否にかかわらず一時プロセスと作業ディレクトリを片付ける。

アプリ、CLI、ワークスペースのパスには空白と日本語を含めます。
DBeaver が起動できない場合や導入・整形が失敗した場合は、その段階でジョブを失敗させます。
実 DBeaver のテストを模擬サーバーに置き換えて成功扱いにする処理はありません。
DBeaver のインスタンス情報や生ログは artifact にせず、段階ごとの固定メッセージだけを失敗レポートへ保存します。
成功時は自作プラグインの更新サイト ZIP だけを保存し、DBeaver 本体やビルド用ターゲットは配布しません。

取得先は [DBeaver 26.2.2 の公式リリース](https://github.com/dbeaver/dbeaver/releases/tag/26.2.2) です。
アセット名・SHA-256 と使用する Actions のコミット SHA は 2026-10-25 に公式リポジトリで確認しました。
Go は 1.27.2、Java は Temurin 25 を指定し、Maven は runner にあるものの版をログへ記録します。

## 現在の確認範囲

2026-10-25 時点で、ローカルの actionlint、Bash 構文検査、CLI と配布内容のテスト、プラグインのビルドとライセンス同梱検査は成功しています。
**GitHub Actions と Windows の実行はまだ確認していません。**
リモートへの push を行っていないため、最初の GitHub Actions 実行結果を確認する必要があります。
この Mac に PowerShell や Windows 用 DBeaver を追加インストールしてはいません。

Windows ジョブが成功しても、対応範囲は Windows x64・CE 26.2.2・標準 SQL 方言です。
実際の DB 接続ごとの整形設定と GUI エディタとの一致、Windows arm64、Linux での実 DBeaver 連携、専用 VS Code 拡張は別途検証が必要です。

## 配布物を手元で作る

リポジトリ直下で実行します。

```sh
go run ./scripts/build-cli.go
go test -v ./scripts/package-cli.go ./scripts/package-cli_test.go
go run ./scripts/package-cli.go
```

ライセンス一覧は `licenses/README.md` を参照してください。
パッケージングは Go の版やビルド設定、ライセンス文書のハッシュを確認し、一覧と合わないバイナリを拒否します。
Go や依存を更新する場合は一覧を再確認してください。
この一覧は実装に基づくライセンス表示の整理で、あらゆる利用条件についての法的な保証ではありません。
