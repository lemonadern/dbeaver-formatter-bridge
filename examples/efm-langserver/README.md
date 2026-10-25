# efm-langserver の設定

[macOS / Linux 用](config-unix.yaml) または [Windows 用](config-windows.yaml) の設定例を使います。
CLI とワークスペースのパスを自分の環境へ置き換え、efm-langserver の設定ファイルにある `languages.sql` へ追加してください。

## 設定ファイル

| 環境 | 配置先 |
|---|---|
| Windows | `%APPDATA%\efm-langserver\config.yaml` |
| macOS / Linux | `$XDG_CONFIG_HOME/efm-langserver/config.yaml`。未設定なら `~/.config/efm-langserver/config.yaml` |

別の場所に置く場合は、`efm-langserver -c /path/to/config.yaml` で指定します。

設定例の `format-stdin: true` により、編集中の文書が CLI の標準入力へ渡されます。
未保存の SQL も整形できます。

## 接続別設定

接続先の方言と整形設定を使う場合は、`format-command` に `--connection "接続ID"` を追加します。
接続 ID の調べ方は [導入手順](../../docs/installation.md#接続-id) を参照してください。

Windows のコマンドは cmd.exe 用に全体を二重引用符で囲むため、オプションは最後の引用符の内側へ追加します。

```yaml
format-command: '""C:\Tools\DBeaver Bridge\dbeaver-format.exe" --workspace "C:\path\to\workspace6" --connection "connection-id""'
```

接続 ID を固定せず、DBeaver で選択中の SQL タブの接続を使う場合は、`--connection` の代わりに `--active-editor` を追加します。
DBeaver で接続済みの SQL タブを選び、外部エディタに戻って整形してください。

```yaml
# macOS / Linux
format-command: '"/path/to/dbeaver-format" --workspace "/path/to/workspace6" --active-editor'
```

```yaml
# Windows
format-command: '""C:\Tools\DBeaver Bridge\dbeaver-format.exe" --workspace "C:\path\to\workspace6" --active-editor"'
```

SQL タブや接続を取得できない場合は整形エラーになり、元の文書を維持します。

## エディタ側の設定

LSP クライアントに次の設定を指定します。
設定の書式は使用するエディタとクライアントに従ってください。

| 項目 | 値 |
|---|---|
| 起動コマンド | `efm-langserver`。設定ファイルを指定する場合は `-c` を付ける |
| 対象の言語 | SQL |
| 初期化オプション | `documentFormatting: true` |

エディタの文書整形操作から `textDocument/formatting` を送ると、efm-langserver が CLI を呼び出し、返された SQL を文書へ反映します。
DBeaver を起動し、`--connection` または `--active-editor` を指定する場合は対象の接続を接続済みにしておきます。
連携の確認範囲は [互換性](../../docs/compatibility.md) を参照してください。
