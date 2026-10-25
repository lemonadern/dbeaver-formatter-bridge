# DBeaver Formatter Bridge

VS Code の SQL 文書を、起動中の DBeaver のフォーマッタで整形します。
保存前の編集内容や、まだファイルとして保存していない SQL も整形できます。

## 導入

1. [CLI と DBeaver プラグインの導入手順](https://github.com/lemonadern/dbeaver-formatter-bridge/blob/main/docs/installation.md)に従い、プラグインを導入して DBeaver を再起動します。
2. OS と CPU に合う CLI を、Windows は `dbeaver-format.exe`、macOS と Linux は `dbeaver-format` として PATH に配置します。
3. VS Code の拡張画面のメニューから「VSIX からのインストール」を選び、配布された `.vsix` を導入します。
4. ユーザー設定の `dbeaverFormatter.workspace` に、起動中の **DBeaver のワークスペース**の絶対パスを指定します。VS Code のプロジェクトフォルダではありません。
5. DBeaver で対象の SQL タブを選び、その接続を接続済みにします。
6. VS Code の SQL 文書で「ドキュメントのフォーマットの方法を指定」から **DBeaver Formatter Bridge** を選びます。

SQL 用の既定フォーマッタに設定すると、その後は通常の整形操作から使えます。
既存の efm や別の SQL フォーマッタを使っている場合も、既定フォーマッタを明示してください。

```json
{
  "dbeaverFormatter.workspace": "C:\\Users\\you\\AppData\\Roaming\\DBeaverData\\workspace6",
  "[sql]": {
    "editor.defaultFormatter": "lemonadern.dbeaver-formatter-bridge",
    "editor.formatOnSave": true
  }
}
```

macOS と Linux では `dbeaverFormatter.workspace` を `/Users/you/.../workspace6` などの絶対パスに置き換えます。
保存時の整形が不要なら `editor.formatOnSave` は省略してください。
拡張はこれらの設定を自動で書き換えません。

## 接続と整形設定

| 設定 | 初期値 | 内容 |
|---|---|---|
| `dbeaverFormatter.workspace` | 未設定 | DBeaver のワークスペースの絶対パス。ユーザー設定で指定 |
| `dbeaverFormatter.connectionMode` | `activeEditor` | 選択中の SQL タブ、接続 ID 指定、グローバル設定の選択 |
| `dbeaverFormatter.connectionId` | 空 | `connection` モードで使う正確な接続 ID |
| `dbeaverFormatter.timeoutMs` | `10000` | 通信のタイムアウト。1 から 120000 ミリ秒 |

`activeEditor` は DBeaver で選択中の SQL タブの接続と整形設定を使います。
DBeaver でタブを選んで VS Code に戻っても、その選択が使われます。
VS Code のファイル名から DBeaver のタブを自動選択する機能はありません。
SQL タブや接続がない場合、未接続の場合、複数ウィンドウの判定が曖昧な場合はエラーになります。

接続 ID を固定する場合は `connectionMode` を `connection` にし、`connectionId` を指定します。
接続先は DBeaver 側であらかじめ接続してください。
`global` にすると、標準 SQL 方言と DBeaver のグローバル設定を使います。

インデントは DBeaver の設定に従います。
VS Code のタブ幅やスペース設定では上書きしません。
入力は UTF-8 で最大 1 MiB、文書全体の整形に対応します。
拡張は改行や末尾の空白を追加処理しませんが、返した編集の改行は VS Code の文書の LF / CRLF 設定に従います。
失敗、キャンセル、処理中の編集があった場合は、その整形結果を適用しません。

## CLI が見つからない場合

拡張は **VS Code の Extension Host が持つ PATH** から CLI を探します。
GUI から起動した VS Code とターミナルでは、PATH が異なることがあります。
CLI のディレクトリを VS Code に渡す PATH に追加し、VS Code 全体を再起動してください。
PATH は絶対パスのディレクトリを使い、Windows は `.cmd` や `.bat` ではなく実際の `.exe` を配置してください。
macOS と Linux では CLI に実行権限も必要です。

`flag provided but not defined: -active-editor` が出る場合は CLI を更新してください。
`update the Formatter Bridge plugin` が出る場合は DBeaver 用プラグインを更新して DBeaver を再起動してください。
DBeaver の未起動や未接続については、表示される CLI の診断に従ってください。

## 対応範囲

VS Code 1.100.0 以降のデスクトップ版を対象とします。
CLI と DBeaver はローカルのマシンで実行します。
Web 版、Remote SSH、Dev Containers、WSL の構成は動作確認の対象外です。
信頼されていないワークスペースでは CLI を実行しません。

CLI、DBeaver 本体、DBeaver 用プラグインは VSIX に含まれません。
専用拡張を使う場合、efm-langserver は不要です。

## ライセンス

[Apache-2.0](LICENSE)
