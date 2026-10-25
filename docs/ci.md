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
5. 分離したユーザーディレクトリとワークスペースで、公式 `dbeaver.exe` と同梱 Java を使って GUI を起動する。
6. CLI に実際の整形要求を送り、起動完了を待つ。
7. Unicode・LF/CRLF・空入力・失敗時の出力・並行要求を検証する。
8. PowerShell と Git Bash から UTF-8 の SQL を渡し、文字列が保たれることを確認する。
9. 成否にかかわらず一時プロセスと作業ディレクトリを片付ける。

CLI とワークスペースのパスには空白と日本語を含めます。
DBeaver の展開先には空白を含む ASCII パスを使います。
Windows の Java ランチャーは起動引数をシステムの ANSI コードページへ変換するため、ワークスペースは percent encode 済みの file URI で Java に渡します。
CLI には元の日本語を含む Windows パスを渡し、読み取りと通信を検証します。
GUI は製品の `.ini` 設定とネイティブライブラリを読み込むため、直接 `java -jar` で代用せず公式ランチャーで起動します。
DBeaver が起動できない場合や導入・整形が失敗した場合は、その段階でジョブを失敗させます。
実 DBeaver のテストを模擬サーバーに置き換えて成功扱いにする処理はありません。
DBeaver のインスタンス情報や生ログは artifact にせず、段階ごとの固定メッセージと、秘匿処理を施した限定的な起動エラー要約を失敗レポートへ保存します。
成功時は自作プラグインの更新サイト ZIP だけを保存し、DBeaver 本体やビルド用ターゲットは配布しません。

取得先は [DBeaver 26.2.2 の公式リリース](https://github.com/dbeaver/dbeaver/releases/tag/26.2.2) です。
アセット名・SHA-256 と使用する Actions のコミット SHA は 2026-10-25 に公式リポジトリで確認しました。
Go は 1.27.2、Java は Temurin 25 を指定し、Maven は runner にあるものの版をログへ記録します。

## 現在の確認範囲

2026-10-25 時点で、ローカルの actionlint、Bash 構文検査、CLI と配布内容のテスト、プラグインのビルドとライセンス同梱検査は成功しています。
**2026-10-25 に [GitHub Actions の全 4 ジョブ](https://github.com/lemonadern/dbeaver-formatter-bridge/actions/runs/40184924311) が成功しました。**
Windows x64 で実 DBeaver CE 26.2.2 の p2 導入・GUI 起動・整形、PowerShell と Git Bash の日本語入力を確認済みです。
Ubuntu・macOS・Windows の CLI テストと、6 種類の CLI 配布 ZIP・チェックサムの検証も成功しています。
成功した Windows runner の Maven は 3.9.16 でした。
配布物は同実行の `cli-distributions` と `plugin-update-site` artifact に保存しています。
artifact の保存期間は 7 日間で、GitHub Releases への公開はまだ行っていません。
この Mac に PowerShell や Windows 用 DBeaver を追加インストールしてはいません。

今回の Windows 統合確認の範囲は Windows x64・CE 26.2.2・標準 SQL 方言です。
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
