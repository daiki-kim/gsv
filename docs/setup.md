# gsv セットアップ

## 1. Google Cloud 側の準備（初回のみ）

1. https://console.cloud.google.com/ でプロジェクトを作成（既存でも可）
2. 「API とサービス」→「ライブラリ」で **Google Sheets API** と **Google Drive API** を有効化
3. 「OAuth 同意画面」を設定
   - Workspace アカウント: ユーザータイプ「内部」
   - 個人アカウント: ユーザータイプ「外部」。テストユーザーに自分を追加
4. 「認証情報」→「認証情報を作成」→「OAuth クライアント ID」→ 種類「デスクトップアプリ」
5. JSON をダウンロードし、次の場所に置く

```bash
mkdir -p ~/.config/gsv
mv ~/Downloads/client_secret_*.json ~/.config/gsv/credentials.json
chmod 600 ~/.config/gsv/credentials.json
```

### リフレッシュトークンの有効期限に注意
「外部」かつ公開ステータス「テスト中」のままだと、リフレッシュトークンが 7 日で失効し、週 1 回ブラウザでの再認可が必要になります（gsv は自動で再認可フローを開始します）。
避けたい場合は OAuth 同意画面の公開ステータスを「本番環境」にしてください。自分だけで使う分には「未検証のアプリ」警告を「詳細」→「移動」で進めれば利用できます。

## 2. ビルド

```bash
cd /Users/dkim/tools/gsv
go build -o gsv ./cmd/gsv
# 任意: PATH の通った場所へ
cp gsv ~/go/bin/   # など
```

## 3. 初回起動

```bash
./gsv
```
ブラウザが開くので Google アカウントでアクセスを許可します。完了すると `~/.config/gsv/token.json`（0600）が保存され、TUI が起動します。

## 4. 共有ドライブをツリーに追加（任意）

`~/.config/gsv/config.yaml`:

```yaml
shared_roots:
  - name: 営業部
    id: 0AbCdEf...   # 共有ドライブ ID（URL の /drive/folders/ 以降）またはフォルダ ID
```
