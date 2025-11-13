# Chores Contributor

家事の報告を管理し、週次でポイント集計を行うAPIサーバー。

## 機能

- 家事の報告（HTTP API + LINE Webhook）
- 週次ポイント集計
- 冪等性保証（重複報告の自動排除）
- ハードコードされたタスク辞書によるポイント換算
- LINEコマンド（`@bot <task> [<option>]`・`@bot me`・`@bot top`・`@bot help`・`@bot 取消`）への即時返信

## セットアップ

### 前提条件

- Go 1.25.4以上
- PostgreSQL 16以上
- Docker & Docker Compose（ローカル開発用）

### 環境変数

`.env`ファイルを作成するか、環境変数を設定してください。

**必須**:
- `DATABASE_URL`: PostgreSQL接続文字列
  - 例: `postgres://app:app@localhost:5432/chores?sslmode=disable`

**オプション**:
- `PORT`: サーバーポート（デフォルト: 8080）
- `LINE_CHANNEL_SECRET`: LINE Webhook署名検証用（LINE Messaging API使用時）
- `LINE_CHANNEL_ID`: LINE返信API呼び出し用トークン（返信を有効化する場合は必須）

`.env`ファイルの例:
```bash
DATABASE_URL=postgres://app:app@localhost:5432/chores?sslmode=disable
PORT=8081
LINE_CHANNEL_SECRET=your-line-channel-secret
LINE_CHANNEL_ID=your-line-channel-token
```

### 起動手順

```bash
# 1. DB起動
docker-compose up -d db

# 2. マイグレーション実行
make migrate-up

# 3. サーバー起動
go run ./cmd/server
```

### 開発環境の起動

```bash
# DB + マイグレーションを一度に実行
docker-compose up
```

## API仕様

詳細は `openapi.yaml` を参照してください。

### 主要エンドポイント

- `POST /events/report` - 家事の報告
`POST /events/report` の例:

```bash
curl -X POST http://localhost:8080/events/report \
  -H "Content-Type: application/json" \
  -d '{
    "group_id": "default-house",
    "user_id": "u1",
    "task": "皿洗い",
    "source_msg_id": "demo-123"
  }'
```

- `GET /houses/{group}/weekly` - 週次集計
- `POST /webhook` - LINE Webhook（返信もここで処理）
- `GET /healthz` - ヘルスチェック
- `GET /debug/vars` - メトリクス

## Makefileコマンド

```bash
make help              # ヘルプ表示
make migrate-up        # マイグレーション実行
make migrate-up-safe   # バックアップ後にマイグレーション実行
make backup            # DBバックアップ作成
make post-chore        # 家事報告のテスト送信
make weekly            # 週次集計の取得
make db-truncate       # テーブル初期化（RESTART IDENTITY）
```

### LINEコマンド一覧

```
@bot 皿洗い         # 家事報告（alias/タイプミス補正あり）
@bot me            # 今週の自分のポイントを返信
@bot top           # 今週のTOP3（準備中）
@bot 取消          # 直前に登録した報告を取り消し
@bot help          # 使い方メッセージ
```

## テスト

```bash
go test ./...
```

## ライセンス

（未設定）

