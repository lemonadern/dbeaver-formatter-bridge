# 導入と削除

## 必要なもの

利用時に必要なのは、既存の DBeaver、対応 CLI、DBeaver プラグインです。
CLI 利用者に Go・Maven・追加 JDK は必要ありません。
efm 連携には別途 efm-langserver とエディタ側の LSP クライアントが必要です。
検証した DBeaver は CE 26.2.2 です。

## プラグイン

ビルドした更新サイト ZIP を DBeaver の「Help → Install New Software」から導入します。
「Add → Archive」で ZIP を選び、「DBeaver Formatter Bridge」feature を選択して導入し、再起動してください。
配布物は開発時点で署名していないため、その扱いは組織の導入方針に従ってください。
更新は同じ手順で新しい ZIP を指定します。
削除は「Help → About DBeaver → Installation Details → Installed Software」から feature を選び、「Uninstall」の後に再起動します。
メニュー名は OS と表示言語で変わります。
`Program Files` などへの書き込み権限や組織のポリシーにより、導入権限が必要になる場合があります。

**この手順による Windows/Linux の実機導入と、既存インストールへの更新・削除は未検証です。**
開発時の macOS 統合検証では、本体を書き換えず、独立した一時 OSGi 構成にプラグインを追加しました。

## CLI

`build/` の対象 OS・アーキテクチャのバイナリをコピーし、`dbeaver-format` または `dbeaver-format.exe` に改名して PATH へ配置します。
macOS/Linux では実行権限も確認します。
未署名の macOS バイナリの配布・公証は未整備です。

ワークスペースは `DBeaverData` 全体ではなく、`.metadata` ディレクトリがある `workspace6` などのディレクトリを指定します。
DBeaver の起動引数 `-data` で変更している場合は、そのパスを使います。
代表的な既定配置は以下ですが、自分の環境の実パスを確認してください。

| OS | ワークスペースの例 |
|---|---|
| Windows | `%APPDATA%\DBeaverData\workspace6` |
| macOS | `~/Library/DBeaverData/workspace6` |
| Linux | `${XDG_DATA_HOME:-~/.local/share}/DBeaverData/workspace6` |

CLI はこのワークスペースの `.metadata/dbeaver-instance.properties` からポートと認証情報を読みます。
認証情報を引数や efm 設定へコピーする必要はありません。
複数のインスタンスが登録されている場合は、誤選択を防ぐためエラーにします。

## 接続 ID

`--connection` はワークスペース内の接続 ID を完全一致で検索します。
DBeaver プロジェクト内の `.dbeaver/data-sources.json` では、`connections` の各キーが接続 ID です。
このファイルには機密情報が含まれる可能性があるため、内容をログやリポジトリへコピーしないでください。
一致する接続が複数プロジェクトにある場合はエラーにします。
DB への自動接続はせず、指定した接続が未接続なら DBeaver で接続するよう通知します。

## エラー時

- インスタンス情報が読めない場合：ワークスペースと DBeaver の起動を確認します。
- 通信できない場合：そのワークスペースで DBeaver が起動しているか確認します。
- bridge command / compatibility エラー：プラグインの導入・再起動・対応版を確認します。
- connection ID / disconnected エラー：指定 ID と接続状態を確認します。
- timeout エラー：大きい入力や外部フォーマッタ設定を確認します。CLI のタイムアウトは DBeaver 内の処理を中断しません。

入力元と同じファイルへ `> input.sql` でリダイレクトすると、シェルが実行前にファイルを切り詰めます。
結果をファイルへ保存する場合は別ファイルを使ってください。
Windows PowerShell 5.1 の標準入出力は UTF-8 を保証しないため、まず efm の stdin 連携か UTF-8 を設定した環境で使用してください。
