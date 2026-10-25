# 通信仕様 v1

## HTTP 接続

CLI は起動中の DBeaver の RPC `POST http://127.0.0.1:PORT/handleCommandLine` を呼びます。
ポート番号と認証情報は、指定したワークスペースの `.metadata/dbeaver-instance.properties` から読み取ります。
要求には `Content-Type: application/json` と `Authorization: Bearer PASSWORD` を設定します。
HTTP プロキシとリダイレクトは使用しません。

## 要求

外側の JSON は DBeaver のコマンド実行形式です。
`args` の 3 要素目に、bridge の要求を JSON 文字列として渡します。

```json
{
  "args": [
    "bridge-format",
    "--request",
    "{\"version\":1,\"sql\":\"select 1\"}"
  ]
}
```

bridge の要求フィールドは次のとおりです。
省略可能なフィールドに `null` は指定できません。

| フィールド | 型 | 必須 | 内容 |
|---|---|---|---|
| `version` | number | はい | 整数 `1` |
| `sql` | string | はい | 整形する SQL。空文字列も指定可能 |
| `connection` | string | いいえ | ワークスペース内の接続 ID。省略時は `BasicSQLDialect` とグローバル設定を使用 |
| `indent` | string | いいえ | 1〜32 文字のスペースまたはタブ。省略時は DBeaver のテキストエディタ設定を使用 |

`connection` を指定した場合は、全プロジェクトから ID が一致する接続を検索し、その方言と接続設定を使用します。
接続は DBeaver で接続済みである必要があります。
同じ ID が複数のプロジェクトにある場合はエラーになります。

CLI は `sql` を UTF-8 で最大 1 MiB に制限します。
プラグイン側の上限は、要求 JSON 全体が 8,388,608 UTF-16 コード単位、`sql` が 1,048,576 UTF-16 コード単位です。

## 応答

### 成功

DBeaver の外側応答は `exitCode: 0` で、`output` に単一の JSON 文字列を返します。

```json
{
  "postAction": "SHUTDOWN",
  "exitCode": 0,
  "output": ["{\"version\":1,\"sql\":\"SELECT 1\"}"]
}
```

内側の `version` は整数 `1`、`sql` は整形結果の文字列です。
`postAction: SHUTDOWN` はコマンド呼び出し側の完了を表し、RPC サーバーとして起動している DBeaver は要求の処理を続けます。

### 失敗

プラグイン内部で検出した失敗は `postAction: ERROR`、`exitCode: 1` で返します。
`output` は、エラーコードを含む単一の JSON 文字列です。

```json
{
  "postAction": "ERROR",
  "exitCode": 1,
  "output": ["{\"version\":1,\"error\":\"connection_not_found\"}"]
}
```

| エラーコード | 内容 |
|---|---|
| `invalid_request` | JSON、フィールドの型、入力上限、インデント指定、またはエディタのインデント幅が不正 |
| `unsupported_version` | `version` が省略されている、または整数 `1` 以外 |
| `connection_not_found` | 指定した接続 ID が見つからない |
| `connection_ambiguous` | 指定した接続 ID が複数のプロジェクトに存在する |
| `connection_disconnected` | 指定した接続が未接続 |
| `formatter_unavailable` | 設定されたフォーマッタを取得できない |
| `formatting_failed` | 整形処理が例外を送出した、または結果が `null` |

CLI は既知のコードを固定の診断メッセージへ変換します。
プラグイン未導入などで bridge 形式のエラーを取得できない場合は、導入状況と互換性の確認を促す診断を返します。
任意のサーバー出力をそのまま診断へ含めません。

## フォーマッタと出力

プラグインは `sql` を文字列として DBeaver の設定済みフォーマッタへ渡します。
外部フォーマッタを設定している場合は、その実装が外部プロセスや一時ファイルを使用することがあります。
フォーマッタが失敗時に元の SQL を返す実装の場合は、その結果を正常な文字列として返します。

CLI が読み取る HTTP 応答の上限は 64 MiB です。
CLI は応答の `exitCode`、`output` の要素数、内側の `version` と `sql` を検証してから整形結果を標準出力へ書きます。
書き込み途中のストリームエラーでは、書き込み済みのバイトは残ります。

## ログと互換性

DBeaver の TRACE ログはコマンド引数を記録するため、要求の SQL がログへ残る場合があります。
機密 SQL を扱う環境では TRACE ログを無効にしてください。

RPC とインスタンス情報は DBeaver の内部 API です。
対応バージョンは [互換性](compatibility.md) を参照してください。
