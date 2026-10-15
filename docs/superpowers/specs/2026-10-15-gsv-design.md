# gsv — Google Sheets Viewer (TUI) 設計書

- 作成日: 2026-10-15
- ステータス: レビュー待ち

## 1. 目的とスコープ

### 目的
普段使っている **非公開の Google スプレッドシート** を、ターミナルから素早く閲覧するための TUI ツール。nvim + ファイルツリー（nvim-tree 等）のような操作感で、左ペインに Drive ツリー、右ペインにシート内容を表示する。

### 成功基準
- 起動から Drive ツリー表示までが体感で待たされない（バイナリ起動 < 0.1s、以降は API 応答待ちのみ）
- マイドライブ / Shared with me / 設定した共有ドライブ配下からスプレッドシートを辿って開ける
- vim のキー操作だけで、ツリー移動・シート閲覧・タブ切替・検索・セル全文確認・再読込ができる
- 読み取り専用スコープのみを要求し、データを書き換えることは一切ない

### スコープ（v1 に含む）
- OAuth（デスクトップアプリ）による自分のアカウントでの認証
- Drive ツリー: My Drive / Shared with me / config 指定の共有ドライブ・フォルダ
- `:find` による Drive 全体のスプレッドシート名検索
- `:e <URL|ID>` による直接オープン
- シートタブ切替、テキスト検索、セル全文ポップアップ、手動リロード
- vim ライクなキー操作とコマンドライン

### 非スコープ（v1 に含まない）
- 編集・書き込み
- ヘッダー行固定、行フィルタ
- クリップボードへのコピー
- ディスクキャッシュ（キャッシュはプロセス内メモリのみ）
- 大きなシートの範囲分割（ページング）取得
- セル書式（色・罫線・結合セル）の再現

## 2. 技術選定

- 言語: Go（インストール済み。単一バイナリ・高速起動）
- TUI: `github.com/charmbracelet/bubbletea`, `lipgloss`（スタイル）, `bubbles`（spinner / textinput を必要に応じて利用）
- Google API: `google.golang.org/api/drive/v3`, `google.golang.org/api/sheets/v4`, `golang.org/x/oauth2`
- 文字幅: `github.com/mattn/go-runewidth`
- 設定: `gopkg.in/yaml.v3`

## 3. 構成

```
/Users/dkim/tools/gsv/
├── cmd/gsv/main.go        エントリ（config 読込 → 認証 → TUI 起動）
├── internal/
│   ├── config/            設定ファイル読込・検証
│   ├── auth/              OAuth フロー・トークン保存/更新
│   ├── gdrive/            Drive ツリー取得（フォルダ + スプレッドシートのみ）
│   ├── gsheets/           タブ一覧・値取得
│   └── ui/                Bubble Tea ルートモデル
│       ├── tree/          左ペイン: Drive ツリー
│       ├── grid/          右ペイン: シート表示
│       ├── cmdline/       ステータスライン + `:` / `/` 入力
│       └── keymap/        vim キーシーケンス → Action の純粋な解釈器
└── docs/
```

### 境界
- `gdrive` / `gsheets` は以下のインターフェースを公開し、`ui` はインターフェースにのみ依存する（テストでフェイクに差し替え可能）。

```go
// gdrive
type Item struct {
    ID       string
    Name     string
    Kind     Kind // KindFolder | KindSpreadsheet
}
type Client interface {
    ListChildren(ctx context.Context, parentID string) ([]Item, error)
    ListSharedWithMe(ctx context.Context) ([]Item, error)
    SearchSpreadsheets(ctx context.Context, name string) ([]Item, error)
}

// gsheets
type Tab struct {
    ID    int64
    Title string
    Rows  int64
    Cols  int64
}
type Client interface {
    GetTabs(ctx context.Context, spreadsheetID string) (title string, tabs []Tab, err error)
    GetValues(ctx context.Context, spreadsheetID, tabTitle string) ([][]string, error)
}
```

- `keymap` は入力キー列を受け取り `Action`（例: `MoveDown{Count:5}`, `NextTab{Count:3}`, `PaneRight`, `EnterCommand`）を返す純粋関数群。UI 状態に依存しない。

## 4. ファイル配置

`~/.config/gsv/`（`$XDG_CONFIG_HOME` が設定されていればそれに従う）

| ファイル | 用途 | 作成者 |
|---|---|---|
| `credentials.json` | GCP で作成した OAuth クライアント（デスクトップアプリ） | ユーザー |
| `token.json` | 取得したトークン。パーミッション `0600` | gsv |
| `config.yaml` | 共有ドライブルート設定（任意。無ければ空設定） | ユーザー |

### config.yaml
```yaml
shared_roots:
  - name: 営業部          # ツリーに表示する名前（必須）
    id: 0AbCdEf...        # 共有ドライブ ID またはフォルダ ID（必須）
```

## 5. 認証

- スコープ: `https://www.googleapis.com/auth/spreadsheets.readonly`, `https://www.googleapis.com/auth/drive.metadata.readonly`
- 初回 / トークン無効時: `127.0.0.1` の空きポートでローカルサーバーを立て、ブラウザで認可 URL を開き（`open` コマンド。失敗時は URL を表示）、リダイレクトで受け取った code をトークンに交換して `token.json` に保存。`state` パラメータを検証する。
- アクセストークン期限切れ: `oauth2.TokenSource` で自動リフレッシュ。リフレッシュ後のトークンが変わっていれば `token.json` に書き戻す。
- 認証は TUI 起動前に完了させる（TUI 中にブラウザ認可は挟まない）。

## 6. Drive ツリー

### ルート（同列表示）
| ルート | 取得方法 |
|---|---|
| My Drive | `'root' in parents` |
| Shared with me | `sharedWithMe = true` |
| config の各 `shared_roots` | `'<id>' in parents` |

### クエリ共通条件
- `trashed = false and (mimeType = 'application/vnd.google-apps.folder' or mimeType = 'application/vnd.google-apps.spreadsheet' or mimeType = 'application/vnd.google-apps.shortcut')`
- ショートカットは `shortcutDetails.targetMimeType` がフォルダ／スプレッドシートのものだけ残し、`targetId` を ID として扱う（表示名はショートカット名）。
- `orderBy=folder,name`、`pageSize=1000`、`nextPageToken` を辿って全件取得
- `supportsAllDrives=true`, `includeItemsFromAllDrives=true`, `corpora=allDrives`
- `fields=nextPageToken, files(id, name, mimeType, shortcutDetails)`

### 遅延ロード
- フォルダ展開時に初めて子を取得。取得は `tea.Cmd` で非同期実行し、取得中はノードに `⟳ loading…` を表示。操作はブロックしない。
- 取得結果はプロセス内でキャッシュ。ツリー上の `r` でそのノードの子を再取得。
- 取得失敗時はノードに `! 読み込み失敗` を表示し、`r` で再試行可能。

### `:find <name>`
- `name contains '<name>' and mimeType = 'application/vnd.google-apps.spreadsheet' and trashed = false`（`'` と `\` はエスケープ）
- 結果はツリー最上段に一時ルート `Search: <name>` として追加（同時に1つまで。再実行で置き換え）。

## 7. シート表示

### 開く流れ
1. `spreadsheets.get`（`fields=properties.title,sheets.properties(sheetId,title,gridProperties)`）でタイトルとタブ一覧を取得
2. 先頭タブを `spreadsheets.values.get`（range=タブ名をシングルクォートで囲みエスケープ、`valueRenderOption=FORMATTED_VALUE`）で取得
3. 他タブは切替時に遅延取得し、タブ単位でキャッシュ
4. 表ペインで `r`: 現在タブを再取得

### 大きなシート
- v1 はタブ全体を一括取得。セル数（行×列）が 200,000 を超える場合はステータスラインに警告を表示するのみ。

### 描画
- 列見出し（A, B, …, Z, AA, …）と行番号を常時表示
- セルは罫線で区切る。列の間に `│`、行の間に `─`、交点は `┼`（見出しとの境界も同様）。行ごとに区切り線が入るため、1 画面に表示できるデータ行数は「(表ペイン高さ − 見出し 2 行) ÷ 2」となる
- 列幅は全列共通の固定値 12 表示桁（行番号列は行数の桁数に合わせる）。超過分は末尾を `…` にして 12 桁に収める
- 全角文字幅は `go-runewidth` で計算（全角文字が境界をまたぐ場合は手前で切り、空白で埋める）
- 値の中の改行・タブはグリッド上では空白に置換（全文ポップアップでは保持）
- カーソルセルは反転表示。カーソルが画面外に出ないよう縦横スクロール
- 選択中セルの全文はステータスラインに表示（端末幅を超える分のみ `…`）。改行を含む長文は `K` / `Enter` のポップアップで確認

## 8. 画面とキー操作

### レイアウト
```
┌ Drive ─────────────┬ 売上管理 ─ [Sheet1] Sheet2  Sheet3 ──────────────────────┐
│ ▾ My Drive         │  │ A          │ B          │ C          │ D          │ │
│   ▾ 仕事           │──┼────────────┼────────────┼────────────┼────────────┤ │
│     ▸ 2025         │ 1│ 日付       │ 顧客       │ 金額       │ 担当       │ │
│     ▤ 売上管理  ◀  │──┼────────────┼────────────┼────────────┼────────────┤ │
│   ▸ 個人           │ 2│ 10/01      │ ABC商事    │ 120,000    │ 田中       │ │
│ ▸ Shared with me   │──┼────────────┼────────────┼────────────┼────────────┤ │
│ ▸ 営業部           │ 3│ 10/03      │ XYZ株式会… │ 98,500     │ 佐藤       │ │
│                    │──┼────────────┼────────────┼────────────┼────────────┤ │
├────────────────────┴──────────────────────────────────────────────────────────┤
│ NORMAL  売上管理 / Sheet1  B3  XYZ株式会社 東京本社                           │
└───────────────────────────────────────────────────────────────────────────────┘
```
- 図の外枠は概念図。実装では外枠を描かず、各ペインの先頭行をタイトル行（ツリー: `Drive`、表: シート名とタブ一覧）とし、ペイン間を縦線 `│` で区切る
- ツリーペイン幅は 30 桁固定（端末幅が 80 未満の場合は 1/3）
- 最下行はステータスライン。`:` / `/` / `?` 入力中はコマンドラインに切り替わる

### モード
NORMAL / COMMAND (`:`) / SEARCH (`/`, `?`) / POPUP（セル全文）

### グローバル
| キー | 動作 |
|---|---|
| `Ctrl-w h` / `Ctrl-w l` / `Ctrl-w w` | ペイン間フォーカス移動 |
| `Ctrl-n` | ツリー表示トグル（非表示時は表ペインにフォーカス） |
| `:q` / `:qa` | 終了 |
| `:e <URL\|ID>` | スプレッドシートを直接開く（URL から `/d/<id>/` を抽出） |
| `:find <name>` | Drive 全体を名前検索 |
| `Esc` | 入力キャンセル / ポップアップを閉じる / 保留中のキー列をクリア |

### ツリーペイン
| キー | 動作 |
|---|---|
| `j` `k`（カウント可）, `gg`, `G` | 移動 |
| `l`, `Enter` | フォルダ: 展開 / スプレッドシート: 開いて表ペインにフォーカス |
| `h` | 展開中なら折りたたみ、そうでなければ親ノードへ |
| `/` → `n` `N` | 読み込み済みノード名の検索（smartcase） |
| `r` | 選択中フォルダの子を再取得 |

### 表ペイン
| キー | 動作 |
|---|---|
| `h` `j` `k` `l`（カウント可）, `gg`, `G`, `0`, `$` | セル移動 |
| `Ctrl-d` / `Ctrl-u` | 半画面スクロール |
| `gt` / `gT` / `{n}gt` | 次タブ / 前タブ / n 番目のタブ |
| `/` `?` → `n` `N` | セル表示値の検索（smartcase、行優先順、末尾で折り返し） |
| `K`, `Enter` | セル全文ポップアップ（`q` / `Esc` で閉じる） |
| `r` | 現在タブを再取得 |

### ステータスライン
モード / スプレッドシート名 / タブ名 / セル番地（例 `B3`）/ セル値（切り詰め）/ メッセージ（エラーは赤）

## 9. エラー処理

| 状況 | 対応 |
|---|---|
| `credentials.json` が無い | TUI を起動せず、配置パスと作成手順（`docs/setup.md`）を案内して終了（exit 1） |
| 初回 / トークン無効 | ブラウザで認可 → `token.json` 保存（§5） |
| アクセストークン期限切れ | 自動リフレッシュ |
| API エラー（403 / 404 等） | ステータスラインに赤字表示。プロセスは継続 |
| 429 / 5xx | 指数バックオフで最大 3 回リトライ後、上記と同様に表示 |
| ネットワーク断 | 対象ノード / タブに失敗表示、`r` で再試行 |
| `config.yaml` の構文・必須項目エラー | 起動時にエラー内容（構文エラーは行番号、必須項目欠落は `shared_roots[i]`）を表示して終了（exit 1） |
| `:` 未知のコマンド | `E492: Not an editor command: <cmd>` をステータスラインに表示 |

## 10. テスト方針

- `keymap`: キーシーケンス → Action のテーブル駆動テスト（`5j`, `gg`, `3gt`, `Ctrl-w l`, `:` 入力, `Esc` による保留キー列クリア）
- `config`: 正常系 / 必須項目欠落 / 構文エラー / ファイル無し
- `gdrive` / `gsheets`: `httptest` のフェイク API サーバーでクエリ組み立て、ページング、ショートカット解決、レンジのエスケープ、リトライを検証
- `ui/tree`, `ui/grid`: フェイククライアントを使い、Bubble Tea の `Update` にメッセージを送って状態遷移（カーソル、展開/折りたたみ、検索ヒット、横スクロール、タブ切替）を検証
- 描画: 全角文字を含む列幅・切り詰めを文字列比較で検証
- 実 API を叩くテストは自動化しない。手動確認手順を `docs/manual-test.md` に記載

## 11. セットアップ（ユーザー作業、`docs/setup.md` に記載）

1. Google Cloud Console でプロジェクトを作成
2. Google Sheets API と Google Drive API を有効化
3. OAuth 同意画面を設定（ユーザータイプ: 外部 or 内部、テストユーザーに自分を追加）
4. 認証情報 → OAuth クライアント ID（種類: デスクトップアプリ）を作成し JSON をダウンロード
5. `~/.config/gsv/credentials.json` として配置
6. `gsv` を起動し、ブラウザで許可

### 注意: リフレッシュトークンの有効期限
- 同意画面が「外部」かつ公開ステータスが「テスト中」の場合、リフレッシュトークンは 7 日で失効し、週 1 回程度ブラウザ再認可が必要になる。gsv はトークン無効を検知したら §5 の認可フローを自動で再実行する。
- 回避策: Workspace アカウントなら「内部」を選ぶ。個人アカウントなら公開ステータスを「本番環境」にする（未検証アプリの警告は出るが自分だけで使う分には許可して進める）。
