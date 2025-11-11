# 小規模運用チェックレポート

## プロジェクト構造

```
chores_contributor/
├── cmd/server/          # エントリーポイント
├── internal/
│   ├── db/             # DB接続
│   ├── http/            # HTTPルーター（chi）
│   ├── repo/            # データアクセス層
│   └── service/         # ビジネスロジック層
├── db/migrations/       # DBマイグレーション（5ファイル）
├── docker-compose.yml   # ローカル開発環境
├── Makefile            # 運用タスク
├── openapi.yaml        # API仕様書
└── report.md           # 実装レポート
```

## ✅ 実装済み項目

### 1. コア機能
- ✅ 家事報告API (`POST /events/report`)
- ✅ 週次集計API (`GET /houses/{group}/weekly`)
- ✅ ハードコードされたタスク辞書によるポイント計算
- ✅ LINE Webhook (`POST /webhook`)
- ✅ ヘルスチェック (`GET /healthz`)
- ✅ メトリクスエンドポイント (`GET /debug/vars`)

### 2. 堅牢性
- ✅ 冪等性保証（`source_msg_id`必須、DB制約）
- ✅ センチネルエラー（`ErrDuplicateEvent`）
- ✅ 入力検証（`DisallowUnknownFields`, Content-Typeガード）
- ✅ タスクキーの正規化（アプリ側でNFKC + trim）
- ✅ 重複検知と明確なレスポンス

### 3. 運用機能
- ✅ アクセスログ（chi middleware.Logger）
- ✅ パニックリカバリ（chi middleware.Recoverer）
- ✅ 優雅なシャットダウン（SIGINT/SIGTERM）
- ✅ JST固定（`time.LoadLocation("Asia/Tokyo")`）
- ✅ DBバックアップ（Makefile）

### 4. インフラ
- ✅ Docker Compose設定
- ✅ マイグレーション管理（migrate）
- ✅ インデックス最適化（カバリングインデックス含む）

## ⚠️ 不足・改善が必要な項目

### 1. ドキュメント不足
- ❌ **README.md**: セットアップ手順、環境変数の説明、起動方法
- ❌ **.env.example**: 環境変数のテンプレート
- ❌ **.gitignore**: ビルド成果物、.env、backup/、pgdata/ の除外

### 2. DB接続設定
- ⚠️ **接続プール設定なし**: `pgxpool`のデフォルト値に依存
  - 推奨: `MaxConns`, `MinConns`, `MaxConnLifetime`の設定

### 3. ヘルスチェック
- ✅ **DB接続チェック追加**: `/healthz`でDB接続を確認（2秒タイムアウト）
  - DB接続エラー時は503を返す

### 4. エラーハンドリング
- ✅ **LINE Webhookのエラーログ追加**: goroutine内のエラーをログ出力

### 5. セキュリティ
- ⚠️ **LINE_CHANNEL_SECRET未設定時の挙動**: 空文字列で署名検証が失敗するが、エラーメッセージが不明確
  - 現状: 403を返す（問題なし）

## 📋 環境変数一覧

### 必須
- `DATABASE_URL`: PostgreSQL接続文字列
  - 例: `postgres://app:app@localhost:5432/chores?sslmode=disable`

### オプション
- `PORT`: サーバーポート（デフォルト: 8080）
- `LINE_CHANNEL_SECRET`: LINE Webhook署名検証用（未設定時は403エラー）
- `LINE_CHANNEL_ID`: LINE返信API呼び出し用トークン（返信が不要なら未設定でも可）

## 🚀 起動手順（想定）

```bash
# 1. 環境変数設定
export DATABASE_URL="postgres://app:app@localhost:5432/chores?sslmode=disable"
export LINE_CHANNEL_SECRET="your-line-secret"  # LINE使用時のみ
export LINE_CHANNEL_ID="your-line-token"   # LINE返信を有効にする場合
export PORT=8081  # オプション

# 2. DB起動・マイグレーション
docker-compose up -d db
make migrate-up

# 3. サーバー起動
go run ./cmd/server
```

## 🔍 運用チェックポイント

### 起動前チェック
- [ ] `DATABASE_URL`が設定されている
- [ ] `LINE_CHANNEL_SECRET`が設定されている（LINE Webhook使用時）
- [ ] `LINE_CHANNEL_ID`が設定されている（LINE返信機能を使う場合）
- [ ] DBが起動している
- [ ] マイグレーションが適用済み

### 起動後チェック
- [ ] `/healthz`が200を返す
- [ ] `/debug/vars`でメトリクスが確認できる
- [ ] ログが正常に出力されている
- [ ] `POST /events/report`で報告が登録できる
- [ ] `GET /houses/{group}/weekly`で集計が取得できる

### 定期チェック
- [ ] ログの確認（エラーがないか）
- [ ] `/debug/vars`でメモリ使用量の確認
- [ ] DBバックアップの実行（`make backup`）

## 📊 パフォーマンス

### インデックス
- ✅ `idx_events_house_created_at`: 週次集計用
- ✅ `idx_events_house_source`: 冪等性チェック用
- ✅ `idx_events_house_created_cover`: カバリングインデックス（週次集計最適化）

### DB制約
- ✅ `events.source_msg_id`: NOT NULL + 長さ制限（1-64文字）

## 🐛 既知の制限事項

1. **DB接続プール**: デフォルト設定のまま（小規模運用では問題なし）
2. **ヘルスチェック**: 簡易的なDB接続チェック（存在しないhouseでクエリ実行）

## ✅ 小規模運用の判定

### 判定: **運用可能** ✅

**理由**:
1. ✅ コア機能が実装済み
2. ✅ 堅牢性の実装が適切
3. ✅ ログ・メトリクス・優雅な停止が実装済み
4. ✅ マイグレーション管理が整備済み
5. ✅ バックアップ機能あり
6. ✅ ヘルスチェックにDB接続確認を追加
7. ✅ LINE Webhookのエラーログを追加
8. ✅ README.mdと.gitignoreを作成

**追加作業（完了済み）**:
1. ✅ README.mdの作成
2. ✅ .gitignoreの作成
3. ✅ LINE Webhookのエラーログ追加
4. ✅ `/healthz`にDB接続チェック追加

**運用開始可能**: ✅ **即座に運用開始可能**

### 運用開始前の最終チェックリスト

- [ ] `DATABASE_URL`環境変数を設定
- [ ] `LINE_CHANNEL_SECRET`環境変数を設定（LINE Webhook使用時）
- [ ] `LINE_CHANNEL_ID`環境変数を設定（LINE返信使用時）
- [ ] DBを起動（`docker-compose up -d db`）
- [ ] マイグレーションを実行（`make migrate-up`）
- [ ] サーバーを起動（`go run ./cmd/server`）
- [ ] `/healthz`でヘルスチェック確認
- [ ] `/debug/vars`でメトリクス確認

### 運用時の注意事項

1. **バックアップ**: マイグレーション前は必ず`make backup`を実行
2. **ログ監視**: エラーログがないか定期的に確認
3. **メトリクス**: `/debug/vars`でメモリ使用量を監視
4. **DB接続**: `/healthz`が503を返した場合はDB接続を確認

### 推奨される今後の改善（任意）

1. DB接続プール設定の最適化（負荷が高くなった場合）
2. より詳細なメトリクス（Prometheus形式など）
3. ログの構造化（JSON形式など）
4. レート制限の実装（LINE経由での想定外トラフィック対策）

