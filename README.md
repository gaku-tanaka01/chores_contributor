# Chores Contributor

家事・購入の報告を管理し、週次でポイント集計を行うAPIサーバー。

## 機能

- 家事/購入の報告（HTTP API + LINE Webhook）
- 週次ポイント集計
- カテゴリ重みの管理
- 冪等性保証（重複報告の自動排除）

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
- `ADMIN_TOKEN`: 管理API用トークン（`/admin/*`エンドポイント用）
- `LINE_CHANNEL_SECRET`: LINE Webhook署名検証用（LINE Messaging API使用時）

`.env`ファイルの例:
```bash
DATABASE_URL=postgres://app:app@localhost:5432/chores?sslmode=disable
PORT=8081
ADMIN_TOKEN=change-me-now
LINE_CHANNEL_SECRET=your-line-channel-secret
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

- `POST /events/report` - 家事/購入の報告
- `GET /houses/{group}/weekly` - 週次集計
- `PUT /admin/houses/{group}/categories/{name}` - カテゴリ重み編集
- `POST /webhook` - LINE Webhook
- `GET /healthz` - ヘルスチェック
- `GET /debug/vars` - メトリクス

## Makefileコマンド

```bash
make help              # ヘルプ表示
make migrate-up        # マイグレーション実行
make migrate-up-safe   # バックアップ後にマイグレーション実行
make backup            # DBバックアップ作成
make post-chore        # 家事報告のテスト送信
make post-buy          # 購入報告のテスト送信
make weekly            # 週次集計の取得
```

## テスト

```bash
go test ./...
```

## ライセンス

（未設定）

