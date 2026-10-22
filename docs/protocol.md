# 通信仕様 v1

CLI は起動中の DBeaver の既存 RPC `POST http://127.0.0.1:PORT/handleCommandLine` を呼びます。
新しいサーバーや公開ポートは追加しません。
認証はインスタンス情報の password を `Authorization: Bearer ...` に設定します。
CLI は HTTP プロキシとリダイレクトを使用しません。

外側の要求は DBeaver の既存形式です。

```json
{"args":["bridge-format","--request","{\"version\":1,\"sql\":\"select 1\"}"]}
```

`--request` は JSON 文字列で、次のフィールドを持ちます。

| フィールド | 内容 |
|---|---|
| version | 整数 1 |
| sql | 必須の文字列。CLI は UTF-8 で最大 1 MiB に制限 |
| connection | 任意。接続 ID。省略時は標準方言とグローバル設定 |
| indent | 任意。1〜32 文字のスペース・タブ。省略時はテキストエディタ設定 |

成功時の外側応答例：

```json
{"postAction":"SHUTDOWN","exitCode":0,"output":["{\"version\":1,\"sql\":\"SELECT 1\"}"]}
```

この `SHUTDOWN` はコマンド呼び出し側の完了を表します。
RPC サーバーとして起動している DBeaver を終了させる操作ではありません。
成功後に別の要求を繰り返せることを実機で確認しています。

プラグイン内部で検出した失敗は `postAction: ERROR`、`exitCode: 1`、単一の JSON 出力 `{"version":1,"error":"CODE"}` を返します。
コードは `invalid_request`、`unsupported_version`、`connection_not_found`、`connection_ambiguous`、`connection_disconnected`、`formatter_unavailable`、`formatting_failed` です。
CLI は既知のコードを固定メッセージへ変換し、任意のサーバー出力を診断にそのまま表示しません。
プラグイン未導入など DBeaver 側の未知エラーは、bridge 導入と互換性を確認する一般エラーになります。

プラグインは SQL ファイルを読み書きせず、文字列をフォーマッタへ渡します。
ただし、DBeaver に外部フォーマッタを設定している場合、その実装が外部プロセスや一時ファイルを使用することがあります。
既存フォーマッタが失敗時に元の SQL を返す場合も、その挙動を継承します。
CLI は応答をすべて検証してから標準出力へ書きます。
応答上限は 64 MiB です。
出力ストリーム自体の書き込みエラーについては、すでに書かれたバイトを取り消せません。

DBeaver の TRACE ログはコマンド引数を記録するため、SQL がログへ残る場合があります。
機密 SQL を扱う環境では TRACE ログを有効にしないでください。
この bridge は SQL・トークン・インスタンス情報を独自のログへ保存しません。
既存 RPC とインスタンス情報は DBeaver の内部 API のため、互換性は版ごとに確認します。
