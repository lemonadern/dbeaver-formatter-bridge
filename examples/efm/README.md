# efm-langserver 連携

`config-unix.yaml` または `config-windows.yaml` を例として、CLI とワークスペースの絶対パスを自分の環境へ置き換えます。
接続別設定を使う場合は `format-command` に `--connection "接続ID"` を追加します。
Windows の例では、コマンド全体を囲む最後の引用符の内側へ追加してください。
ユーザーの設定ファイルをこのリポジトリの例で上書きせず、既存の `languages.sql` へ統合してください。

設定場所の例：

- Windows：`%APPDATA%\efm-langserver\config.yaml`
- macOS / Linux：`~/.config/efm-langserver/config.yaml`（または `-c` で明示）

エディタ側は既存の LSP クライアントから `efm-langserver` を起動し、SQL の整形要求を efm に送ります。
`format-stdin: true` により、未保存の文書も CLI の標準入力に渡されます。
`format-command` に入力 SQL や `${INPUT}` を埋め込む必要はありません。
設定例では実行ファイルをフルパスで指定し、PATH がエディタから見えない問題を避けます。

macOS では実際の efm 0.0.57 に stdio LSP の要求を送り、正常な整形と CLI 失敗時の空の編集結果を確認しました。
Windows の設定例は cmd.exe 用の二重引用符を使用していますが、実機確認は未実施です。
エディタ固有の起動設定は利用する LSP クライアントに合わせてください。
