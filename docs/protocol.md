# 通信仕様

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
| `version` | number | はい | 通常は整数 `1`。選択中の SQL エディタを使う場合は整数 `2` |
| `activeEditor` | boolean | v2 で必須 | v2 では `true`。v1 では省略または `false` |
| `sql` | string | はい | 整形する SQL。空文字列も指定可能 |
| `connection` | string | いいえ | ワークスペース内の接続 ID。v1 で省略時は `BasicSQLDialect` とグローバル設定を使用 |
| `indent` | string | いいえ | 1〜32 文字のスペースまたはタブ。省略時は DBeaver のテキストエディタ設定を使用 |

`connection` を指定した場合は、全プロジェクトから ID が一致する接続を検索し、その方言と接続設定を使用します。
接続は DBeaver で接続済みである必要があります。
同じ ID が複数のプロジェクトにある場合はエラーになります。

`version: 2` は `activeEditor: true` 専用で、`connection` と同時には指定できません。
選択中の SQL エディタから接続を取得し、その方言と接続設定を使います。
UI スレッドで接続を確定してから、整形を実行します。
UI への要求が 2 秒以内に完了しない場合は `ui_timeout` を返します。
SQL エディタや接続がない場合にグローバル設定へ切り替えることはありません。

```json
{"version":2,"activeEditor":true,"sql":"select 1"}
```

新モードだけ v2 を使うことで、旧プラグインが `activeEditor` を無視してグローバル設定で成功することを防ぎます。
旧プラグインからの v1 `unsupported_version` 応答は、更新が必要なエラーとして扱います。

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

内側の `version` は要求と同じ整数 `1` または `2`、`sql` は整形結果の文字列です。
CLI は要求と異なるバージョンの成功応答を拒否します。
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
| `unsupported_version` | `version` が省略されている、または整数 `1`、`2` 以外 |
| `active_editor_unavailable` | 選択中のエディタがない、または SQL エディタではない |
| `active_editor_no_connection` | SQL エディタに接続が割り当てられていない |
| `active_window_ambiguous` | 対象ウィンドウを取得できず、複数のウィンドウが開いている |
| `ui_unavailable` | Workbench や UI を利用できない |
| `ui_timeout` | UI スレッドからの応答待ちが 2 秒を超えた |
| `connection_not_found` | 指定した接続 ID が見つからない |
| `connection_ambiguous` | 指定した接続 ID が複数のプロジェクトに存在する |
| `connection_disconnected` | 指定した接続が未接続 |
| `formatter_unavailable` | 設定されたフォーマッタを取得できない |
| `formatting_failed` | 整形処理が例外を送出した、または結果が `null` |

受理した要求のエラー応答にも、要求と同じ `version` を返します。
要求のバージョンを受理できない場合は `version: 1` で返します。

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
