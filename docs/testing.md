# 検証

CLI の単体テストは `cli/` で実行します。
HTTP テストはローカルのテスト用リスナーを使用します。

```sh
go test -race -timeout 30s ./...
go vet ./...
```

認証、リダイレクト拒否、プロキシ不使用、タイムアウト、JSON 検証、Java properties、UTF-8、入力上限、失敗時の空の標準出力を検証します。
プラグインのビルドはリポジトリ直下で `go run ./scripts/build-plugin.go` を実行します。

## CLI と DBeaver の統合テスト

プラグインを読み込んだ DBeaver に対して実行します。
日本語・絵文字・引用符・バックスラッシュ、LF/CRLF、空入力、接続 ID 不明、タイムアウト、並行したインデント指定を検証します。
DBeaver の設定は変更しません。

```sh
go run ./scripts/test-bridge.go --cli /absolute/path/dbeaver-format --workspace /absolute/path/workspace6
```

## efm を含む統合テスト

対象の DBeaver にプラグインを読み込ませ、GUI として起動した後に実行します。
この検証用スクリプトは POSIX シェル用です。
Windows の検証は別途必要です。

```sh
go run ./scripts/test-efm.go \
  --efm /absolute/path/efm-langserver \
  --cli /absolute/path/dbeaver-format \
  --workspace /absolute/path/workspace6
```

スクリプトは SQL の直接 CLI 出力を基準に、実 efm に initialize、didOpen、textDocument/formatting を送り、未保存の SQL が同じ結果になることを確認します。
存在しないワークスペースによる CLI 失敗時に、efm が編集を返さないことも確認します。
エディタの画面操作を代替するテストではなく、efm までの LSP 経路の検証です。

実機確認では、個人の設定を避けるため一時ワークスペースと一時 OSGi 構成を使用しました。
インストール済み DBeaver のコードと GUI を使っていますが、通常インストールに ZIP を導入する手順の検証とは区別します。

## 2026-10-23 の結果

CLI 単体テスト・race 検出・vet、URI のパステスト、Tycho ビルド、6 種類の CLI クロスビルドが成功しました。
macOS arm64 の GUI に対する `test-bridge.go` と実 efm 0.0.57 に対する `test-efm.go` が成功しました。
空白・日本語を含む CLI の配置とワークスペース指定でも、同じ統合テストが成功しました。
並行整形は、各インデントを指定した単独実行と結果が一致することを確認しています。

p2 director では、既存 DBeaver を変更せず、一時環境への導入、更新サイト ZIP を使った旧修飾子から新修飾子への更新、削除を確認しました。
配布 ZIP は独自 bundle・feature と p2 メタデータだけを含み、DBeaver 本体の JAR を含みません。
GUI メニューを使った導入や DB 接続別設定の一致は、この結果には含めません。

手動入力例には `tests/fixtures/strings.sql` を使用できます。

## まだ必要な確認

- 対象 DB の接続済み ID、方言、接続別設定と GUI エディタの整形結果の一致。
- 未接続の実接続 ID と複数プロジェクトでの ID 重複。
- Windows/Linux の実行、efm、p2 導入・更新・削除。
- 各 OS で日本語や空白を含む配置パス、端末の文字コード。

## 配布と CI の検証

[ci.md](ci.md) に実行対象と確認済みの範囲を記載しています。
配布 ZIP の検証は 6 種類のバイナリをビルドした後に実行します。

```sh
go test -v ./scripts/package-cli.go ./scripts/package-cli_test.go
go run ./scripts/ci/verify-site.go
```
