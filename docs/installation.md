# 導入と削除

DBeaver、DBeaver Formatter Bridge のプラグイン、CLI を用意します。
対応する環境は [互換性](compatibility.md) を参照してください。
エディタから使う場合は、[efm-langserver の設定例](../examples/efm/README.md) も参照してください。

## 配布物の取得

[CI の成果物](ci.md#配布物を取得する) から、`plugin-update-site` と `cli-distributions` を取得して展開します。
前者にはプラグイン導入用 ZIP、後者には OS と CPU ごとの CLI 配布 ZIP が入っています。
CLI の配布 ZIP は、自分の環境に合うものを選んで展開してください。

## DBeaver プラグイン

1. DBeaver の **Help → Install New Software** を開きます。
2. **Add → Archive** でプラグイン導入用 ZIP を選びます。
3. **DBeaver Formatter Bridge** を選んで導入し、DBeaver を再起動します。

更新する場合も、新しい ZIP を指定して同じ手順を実行します。
削除する場合は **Help → About DBeaver → Installation Details → Installed Software** で DBeaver Formatter Bridge を選び、**Uninstall** 後に再起動します。
メニュー名は OS と表示言語で異なります。

## CLI

CLI 配布 ZIP 内の `dbeaver-format`（Windows は `dbeaver-format.exe`）を PATH に配置します。
macOS と Linux で実行権限がない場合は、`chmod +x dbeaver-format` を実行してください。
配布済み CLI の実行に Go、Maven、追加 JDK は不要です。

DBeaver を起動し、ワークスペースを指定して SQL を標準入力へ渡します。

```sh
dbeaver-format --workspace "/path/to/workspace6" < input.sql
```

ワークスペースには、`.metadata` ディレクトリがある場所を指定します。
DBeaver の起動引数 `-data` で配置を変更している場合は、そのパスを使います。

| OS | ワークスペースの配置例 |
|---|---|
| Windows | `%APPDATA%\DBeaverData\workspace6` |
| macOS | `~/Library/DBeaverData/workspace6` |
| Linux | `$XDG_DATA_HOME/DBeaverData/workspace6`。未設定なら `~/.local/share/DBeaverData/workspace6` |

Git Bash では、Windows 形式のパスも指定できます。

```sh
dbeaver-format.exe --workspace "C:/Users/NAME/AppData/Roaming/DBeaverData/workspace6" < input.sql
```

入力ファイルと同じファイルへ出力をリダイレクトすると、シェルが実行前にそのファイルを切り詰めます。
保存する場合は別のファイルへ出力してください。

## 接続 ID

接続先の SQL 方言と設定を使うには、DBeaver で接続してから `--connection ID` を指定します。
接続 ID は DBeaver プロジェクトの `.dbeaver/data-sources.json` にある `connections` のキーで確認できます。
表示名ではなく、この ID を完全一致で指定します。

```sh
dbeaver-format --workspace "/path/to/workspace6" --connection "connection-id" < input.sql
```

同じ ID が複数プロジェクトにある場合や、指定した接続が切れている場合はエラーになります。
接続 ID を省略した場合の設定は [README](../README.md#利用) を参照してください。

## エラーへの対処

| エラーの内容 | 確認すること |
|---|---|
| インスタンス情報を読めない | `--workspace` のパスと、そのワークスペースでの DBeaver の起動状態 |
| DBeaver に接続できない | DBeaver が終了していないか |
| bridge command / compatibility | プラグインの導入、DBeaver の再起動、[対応版](compatibility.md) |
| connection ID / disconnected | 接続 ID と DBeaver 上の接続状態 |
| timeout | 入力サイズ、外部フォーマッタの設定、`--timeout` の値 |

CLI のタイムアウトは DBeaver 内の整形処理を中断しません。

## 文字コード

CLI の入力は UTF-8 です。
PowerShell からパイプで渡す場合は、外部プログラムへの入出力を UTF-8 に設定します。

```powershell
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)
[Console]::OutputEncoding = $OutputEncoding
"select '日本語';" | dbeaver-format.exe --workspace "C:\path\to\workspace6"
```

ファイルを読む際も、そのファイルの文字コードに合わせてください。
