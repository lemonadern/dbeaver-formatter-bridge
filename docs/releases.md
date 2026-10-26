# リリース手順

製品バージョンは `VERSION` で管理します。
CLI、DBeaver プラグイン、VS Code 拡張を同じ通常の `X.Y.Z` に揃えます。
通信プロトコルの v1 / v2 は製品バージョンとは別です。
現在の Go モジュールパスでは major 0 と 1 のみを扱います。major 2 以降は `/vN` へのモジュール移行が必要なので、準備とタグ作成の前に拒否します。

## 版を準備する

リポジトリ直下で実行します。
以下の `0.2.0` は例なので、公開する版を明示してください。

```sh
go run ./scripts/release.go prepare 0.2.0
go run ./scripts/release.go check
git diff
```

`VERSION`、CLI の生成 Go 定数、Maven の各 POM、OSGi bundle と feature、VSIX の package.json と lockfile をまとめて更新します。
ほかの依存パッケージの版は変更しません。
`prepare` はコミットや push を行わないので、差分を確認して PR にします。
Go、Git を利用し、版を自動推測することはありません。

CLI は `dbeaver-format --version` で製品の版を表示します。
Git コミット情報がビルドに含まれる場合は併記します。
`go install` から入れた場合も生成済み Go 定数を使うため、linker flags を指定せずに製品の版を確認できます。

## マージ後に公開を開始する

版更新の PR をマージし、通常 CI が成功した後に実行します。

```sh
git switch main
git pull --ff-only origin main
go run ./scripts/release.go tag 0.2.0
```

`tag` はリモートの main を取り直し、作業ツリーが clean で、main が最新の origin/main と一致することを確認します。
要求した版と生成済み定義が一致し、既存の版より大きいことも確認します。
その後 `v0.2.0` と `cli/v0.2.0` の annotated tag を同じコミットに作り、**二つのタグを atomic push** します。
このコマンドはリモートを更新し、Release workflow を開始します。
既存タグや公開済みタグの付け替えは行いません。

Go のモジュールが `cli/` にあるため、CLI 用のタグは別に必要です。
製品用のタグだけが Release workflow の入口になり、CLI 用タグでは別の Release を作りません。
プレリリースや署名、Marketplace 公開はこの手順の対象外です。

## 自動で行うこと

Release workflow はタグ、版定義、既存 Release を確認します。
新規の場合は通常 CI と共通のビルドとテストを実行し、公開された Go モジュールの `go install` も確認します。
テストを通した CLI 六つの ZIP、p2 ZIP、VSIX と、集約した `SHA256SUMS` の9ファイルを Release の下書きに添付します。
再取得後のハッシュも照合してから正式公開します。
公開ジョブでは配布物をビルドし直しません。

CLI の ZIP 名は `dbeaver-format-<version>-<OS>-<CPU>.zip` です。
プラグインは `formatter-bridge-update-site-<version>.zip`、拡張は `dbeaver-formatter-bridge-<version>.vsix` です。
初回の手動リリース v0.1.0 の添付名は変更しません。

## 失敗した場合

Actions の失敗した段階を確認し、タグを付け替えずに再実行します。
公開済みの添付ファイルを `--clobber` で置き換えないでください。

下書きには元の検証コミット、run / attempt、artifact ID、添付予定ファイルのハッシュを記録します。
復旧ではその artifact から不足分だけを補い、現在の attempt の同名ファイルを推測して使いません。
既存添付物が想定と違う場合や、復旧に必要な artifact が期限切れの場合は停止します。
元の検証済み成果物を失った場合も、自動で再ビルドした別のバイト列へ置き換えません。

公開済みなら、記録と添付物を照合して変更せず完了します。
初回の手動 v0.1.0 のように機械用の復旧記録がないリリースは、自動更新せず停止します。
版や添付内容を修正して配布する場合は、新しい版を準備してください。

PR の検証は、版更新コマンド、一時 Git リポジトリ、模擬 API、実配布物の集約を対象とします。
本番の新タグ起動と Release 公開は、マージ後の次回リリースで初めて実行します。
