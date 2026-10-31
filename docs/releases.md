# リリース手順

DBeaver プラグイン、CLI、VS Code 拡張は、同じリポジトリから別々の版と Release で公開します。
通信プロトコルの v1 / v2 は各製品の版とは別です。

| 対象 | 版の管理元 | タグ | 添付物 |
| --- | --- | --- | --- |
| plugin | `plugin/VERSION` | `plugin/vX.Y.Z` | オンライン p2 の個別ファイル、オフライン ZIP、SHA256SUMS |
| cli | `cli/VERSION` | `cli/vX.Y.Z` | 六つの OS / CPU 別 ZIP、SHA256SUMS |
| vscode | `extensions/vscode/VERSION` | `vscode/vX.Y.Z` | VSIX、SHA256SUMS |

## 変更時にリリースノートを書く

利用者に影響する変更では、同じ PR で `releases/<component>/unreleased.md` も更新します。
変更した挙動、導入方法、互換性、既知の制約を記載し、共通の変更は影響する対象それぞれに記載します。
[AGENTS.md](../AGENTS.md) にもこの運用を定めています。
自動検査は空原稿や雛形の残りを拒否しますが、変更点の記載漏れは PR レビューでも確認します。

## 対象の版を準備する

リポジトリ直下で実行します。
`0.2.0` は例です。

```sh
go run ./scripts/release.go prepare plugin 0.2.0
go run ./scripts/release.go check
git diff
```

`prepare` は指定した対象の版定義だけを更新し、未リリース原稿から `releases/plugin/0.2.0.md` を作成します。
完成した未リリース原稿は雛形に戻し、次の変更を書ける状態にします。
既にある版原稿は上書きしません。
`Summary`、`Changes`、`Compatibility` を確認して雛形を埋め、版定義と一緒に PR にします。
コミット、タグ、push は自動では行いません。
`check plugin` は対象一つ、対象省略の `check` は全対象の版定義を検査します。
初回の旧 `0.1.0` を除き、`check` は版付き原稿も必須とするため、通常の版更新 PR の CI でも未完成の原稿を検出します。

CLI は `dbeaver-format --version` で版を表示します。
版定数をコミットするため、`go install` でもビルド時の linker flags は不要です。
現行 Go モジュールパスでの CLI は major 0 / 1 を扱い、2 以上は `/vN` 移行を必要として拒否します。
この Go 固有の制約は plugin と vscode には適用しません。

## マージ後に公開を開始する

版更新の PR をマージし、通常 CI の成功と公開する原稿を確認した後、許可された公開作業として実行します。

```sh
git switch main
git pull --ff-only origin main
go run ./scripts/release.go tag plugin 0.2.0
```

`tag` は main が最新の origin/main と一致すること、作業ツリーが clean であること、対象の版定義と原稿、既存タグより大きい版であることを確認します。
その後、対象の annotated tag 一つを作成して push し、Release workflow を開始します。
CLI は `cli/vX.Y.Z` が Go モジュール用のタグも兼ねるため、ルートの `vX.Y.Z` を別に作りません。
公開済みの旧 `v0.1.0` と `cli/v0.1.0` は維持し、plugin と vscode の最初の独立版も `0.1.0` より大きくします。
Marketplace、プレリリース、署名はこの手順の対象外です。

## 検証と公開

Release workflow は対象と版、タグ、原稿、既存 Release を確認します。
新規なら通常 CI と共通のビルドと試験を行い、CLI のリリースではタグ経由の `go install ...@vX.Y.Z` も検証します。
検証済みの対象の配布物と `SHA256SUMS` を下書きへ添付し、再取得したバイト列を照合してから公開します。
公開ジョブで再ビルドしません。
Release 本文は対象版の原稿を使い、別セクションに復旧用の機械記録を保持します。

Windows の実環境試験では [compatibility.json](../releases/compatibility.json) に固定した公開済み最小対応版との組合せも確認します。
対応下限の変更は、固定 URL とチェックサム、対応範囲の記載、試験を同じ PR で変更してください。
対象間の版が一致することを要求する設計にはしません。

## プラグイン更新サイト

利用者が登録する固定 URL は、初回配信後に次を使います。

```text
https://lemonadern.github.io/dbeaver-formatter-bridge/updates/
```

Pages には composite の索引だけを置き、正式公開済みの `plugin/vX.Y.Z` Release を参照します。
各 Release には平坦化した p2 メタデータと JAR を個別添付します。
JAR は元の配布物と同じバイト列を使い、取得先はその Release の固定 URL にします。
取得先のタグ部分は `.../releases/download/plugin/vX.Y.Z/` とし、スラッシュを保持します。
p2 の composite に渡す前に `%2F` へ変換すると Equinox が二重にエンコードするため、この変換は行いません。
CLI と VSIX の Release、下書き、プレリリース、旧形式の `v0.1.0` は索引に入りません。
リポジトリ全体の `releases/latest` は対象別の導入先として使いません。

オフライン ZIP には従来どおり `plugins/` と `features/` を含め、ローカルの取得先を維持します。
オンライン用に加工したメタデータを ZIP に混ぜません。
既存 ZIP で入れた正式 `0.1.0` からは、固定 URL を追加して新版へ更新します。
開発版 `0.1.0.<timestamp>` から正式 `0.1.0` は降順になるため、従来のアンインストール後の入れ直し手順を使います。

初回の公開前に管理者が Settings → Pages の Source を GitHub Actions にし、`github-pages` environment が対象タグからのデプロイを許可することを確認してください。
workflow は設定を自動変更しません。
Release 公開後、全ての公開済み plugin Release の記録と添付物を検証して索引を再生成し、直列化した Pages ジョブでデプロイします。
デプロイ後は固定 URL の索引が期待値と一致し、参照する Release の全配布物が HTTP 経由でチェックサムと一致することを確認します。

## 失敗からの復旧

タグを付け替えず、Actions の失敗した段階を確認して再実行します。
下書きでは記録した元 artifact ID とハッシュを使って不足分だけを補完します。
同名添付物のハッシュ不一致や、必要な元 artifact の期限切れは停止します。
公開済みファイルの置換や `--clobber` は使いません。
元 artifact の期限切れだけで、記録と一致する公開済み配布物を失敗させることはありません。
機械記録のない旧 `v0.1.0` は自動修復の対象外です。

プラグインの Release 公開後に Pages の生成、デプロイ、HTTP 配信確認が失敗した場合も、同じ実行を再実行します。
既存 Release の検証が済んでも後段の Pages ジョブへ進みます。
Pages の反映待ちは上限付きで再試行し、古い索引を成功扱いにしません。
新しい索引はデプロイ時点の公開済み Release を列挙して作り、古い実行から新版が消えることを防ぎます。

PR CI は本番タグや Release を作りません。
本番タグからの初回起動、実 Release の公開、Pages の初回配信はマージ後の公開作業で確認します。
