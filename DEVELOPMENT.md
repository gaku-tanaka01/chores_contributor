# DEVELOPMENT.md

Chores Contributor を開発・運用する際の技術的な情報をまとめています。ユーザー向けの利用方法は `README.md` を参照してください。

## 前提条件

- Go 1.25.4 以上
- PostgreSQL 16 以上
- Docker & Docker Compose（ローカル開発・検証用）

## 環境変数

`.env` ファイルを利用するか、適宜環境変数を設定してください。

| 変数名 | 必須 | 用途 |
| ------ | ---- | ---- |
| `DATABASE_URL` | ✅ | PostgreSQL 接続文字列（例: `postgres://app:app@localhost:5432/chores?sslmode=disable`） |
| `PORT` | ❌ | HTTP サーバーポート（デフォルト: 8080） |
| `LINE_CHANNEL_SECRET` | ❌ | LINE Webhook の署名検証に利用 |
| `LINE_CHANNEL_ID` | ❌ | LINE 返信 API に利用（返信を有効化する場合は必須） |

`.env` の例:

```bash
DATABASE_URL=postgres://app:app@localhost:5432/chores?sslmode=disable
PORT=8081
LINE_CHANNEL_SECRET=your-line-channel-secret
LINE_CHANNEL_ID=your-line-channel-token
```

## ディレクトリ構成

```
.
├── cmd/                 # エントリーポイント（HTTPサーバーなど）
├── internal/            # ドメインロジックとアプリケーション層
├── db/                  # マイグレーションやシードデータ
├── supabase/            # Supabase 関連の設定
├── testdata/            # テスト用データ
├── openapi.yaml         # API 仕様書
├── docker-compose.yml   # ローカル開発用コンテナ定義
└── Makefile             # 開発コマンド
```

## セットアップと起動

```bash
# 1. DB を起動
docker-compose up -d db

# 2. マイグレーションを適用
make migrate-up

# 3. アプリケーションを起動
go run ./cmd/server
```

ローカルで一括起動する場合:

```bash
docker-compose up
```

## 開発フローのヒント

- `make migrate-up-safe` でバックアップ付きマイグレーションを適用できます。
- `make post-chore` で LINE Bot を介さずに報告 API をテストできます。
- OpenAPI 定義を更新した際は `make generate-client`（未定義の場合は追加を想定）でクライアント生成を行う運用を想定しています。
- 開発時は `LOG_LEVEL=debug` を指定すると詳細ログを確認できます。

## Makefile コマンド

```bash
make help              # ヘルプ表示
make migrate-up        # マイグレーション実行
make migrate-up-safe   # バックアップ後にマイグレーション実行
make backup            # DB バックアップ作成
make post-chore        # 家事報告のテスト送信
make weekly            # 週次集計の取得
make db-truncate       # テーブル初期化（RESTART IDENTITY）
```

## API 検証

- 主要エンドポイントの詳細は `openapi.yaml` を参照してください。
- `POST /events/report` や `GET /houses/{group}/weekly` を中心に、curl などで手軽に動作確認できます。

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

## テスト

```bash
go test ./...
```

