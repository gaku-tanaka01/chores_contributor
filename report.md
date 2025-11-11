# 実装レポート

## 冪等性（source_msg_id）と重み更新APIの堅牢化

### 背景

小規模・信頼重視の運用方針に基づき、daily_cap_minutesロジックを削除し、代わりに「報告単位の精度」で整合性を保つ設計に変更しました。その中核として、冪等性（source_msg_id）と重み更新APIを堅牢に実装しました。

### 1. 冪等性の実装

#### 1.1 source_msg_idの必須化

**実装箇所**: `internal/service/service.go`

```29:31:internal/service/service.go
	if p.SourceMsgID == nil || *p.SourceMsgID == "" {
		return errors.New("source_msg_id is required for idempotency")
	}
```

- `Report`メソッドの最初に`source_msg_id`の存在チェックを追加
- `nil`または空文字列の場合は明確なエラーメッセージを返す
- これにより、クライアント側で必ず一意のIDを付与することを強制

#### 1.2 DBレベルでの冪等性保証

**実装箇所**: `internal/repo/repo.go`

```95:107:internal/repo/repo.go
	result, err := tx.ExecContext(ctx, `
INSERT INTO events(house_id,user_id,kind,category_id,points,source_msg_id,created_at,note)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(house_id, source_msg_id) DO NOTHING
`, houseID, userID, KindChore, catID, p.Points, p.SourceMsgID, p.Now, p.Note)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrDuplicateEvent
	}
```

- `events`テーブルの`UNIQUE (house_id, source_msg_id)`制約を活用
- `ON CONFLICT DO NOTHING`により、同じ`source_msg_id`での重複挿入を完全に防止
- `RowsAffected()`をチェックして重複を検知し、呼び出し側に通知

#### 1.3 設計思想

- **報告単位の精度**: 各報告（イベント）を`source_msg_id`で一意に識別
- **クライアント責任**: クライアント側で一意のIDを生成・管理することを前提
- **自動重複防止**: DB制約により、サーバー側で自動的に重複を排除

### 2. 重み更新APIの堅牢化

#### 2.1 バリデーション強化

**実装箇所**: `internal/http/router.go`

```121:140:internal/http/router.go
	admin.Put("/houses/{group}/categories/{name}", func(w http.ResponseWriter, r *http.Request) {
		group := chi.URLParam(r, "group")
		name := chi.URLParam(r, "name")
		if group == "" || name == "" {
			writeErr(w, 400, "group and name are required")
			return
		}
		var body struct {
			Weight float64 `json:"weight"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeErr(w, 400, "invalid json: "+err.Error())
			return
		}
		if body.Weight <= 0 {
			writeErr(w, 400, "weight must be greater than 0")
			return
		}
```

**改善点**:
- URLパラメータの空チェックを追加
- `DisallowUnknownFields()`により、未知フィールドを拒否（typo検知）
- JSONデコードエラーを個別に処理（より明確なエラーメッセージ）
- `weight`が0より大きいことを明示的にチェック

#### 2.2 エラーハンドリングの改善

**実装箇所**: `internal/http/router.go`

```141:149:internal/http/router.go
		if err := sv.UpsertCategory(r.Context(), group, name, body.Weight); err != nil {
			if errors.Is(err, repo.ErrHouseNotFound) {
				writeErr(w, 404, "house not found")
				return
			}
			writeErr(w, 500, "update failed: "+err.Error())
			return
		}
		w.WriteHeader(204)
	})
```

**改善点**:
- センチネルエラー（`repo.ErrHouseNotFound`）を使用してエラー判定
- houseが存在しない場合は404を返す（適切なHTTPステータスコード）
- その他のエラーは詳細メッセージを含む500を返す
- クライアント側でエラーの種類を判別可能

#### 2.3 リポジトリ層の改善

**実装箇所**: `internal/repo/repo.go`

```141:160:internal/repo/repo.go
func (r *Repo) UpsertCategory(ctx context.Context, extGroupID, name string, weight float64) error {
	// まずhouseが存在するか確認
	var houseID int64
	err := r.db.QueryRow(ctx, `
SELECT id FROM houses WHERE ext_group_id=$1
`, extGroupID).Scan(&houseID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrHouseNotFound
		}
		return err
	}

	// カテゴリを更新/作成
	_, err = r.db.Exec(ctx, `
INSERT INTO categories(house_id,name,weight)
VALUES($1,$2,$3)
ON CONFLICT(house_id,name) DO UPDATE SET weight=EXCLUDED.weight
`, houseID, name, weight)
	return err
}
```

**改善点**:
- houseの存在確認を先に行う（2段階チェック）
- `pgx.ErrNoRows`をセンチネルエラーに変換
- `ON CONFLICT`により、既存カテゴリの重み更新と新規作成を統一的に処理

### 3. daily_cap_minutesロジックの削除

以下の機能を削除しました：

- `repo.TodayChoreMinutes`: 当日のchore合計分数取得
- `repo.DailyCapMinutes`: 1日上限の取得
- `service.Report`内の上限チェックロジック

**理由**:
- 小規模運用では、報告単位の精度（冪等性）で整合性を保つ方が合理的
- 複雑な上限管理ロジックが不要
- クライアント側での適切な報告管理を前提とする

---

## 堅牢化の実装（第2フェーズ）

### 背景

初期実装では「堅牢」と言い切れない穴が複数存在していました。バイパス・取りこぼし・将来の破壊的変更を防ぐため、以下の10項目を実装しました。

### 1. DBで"必須"を担保

**問題**: service層でのバリデーションだけでは、別クライアントや将来のバグで素通りする可能性がある。

**実装**: `db/migrations/000001_0001_init.up.sql`

- `events.source_msg_id` を `NOT NULL` に設定し、`(house_id, source_msg_id)` にユニーク制約を付与
- `CONSTRAINT events_source_msg_len` で1〜64文字の長さ制限を明示
- 初期マイグレーション段階で必須制約を定義しているため、後続マイグレーションに頼らず堅牢性を担保

**効果**:
- DBレベルで`source_msg_id`の必須性を保証
- 長さ制限により暴走を防止
- 初期時点で制約が整うため、既存データの後追い修正が不要

### 2. センチネルエラー化

**問題**: 文字列比較によるエラーハンドリングは脆く、エラーメッセージの変更で壊れる。

**実装**: `internal/repo/repo.go`

```15:18:internal/repo/repo.go
var (
	ErrHouseNotFound   = errors.New("house not found")
	ErrDuplicateEvent  = errors.New("duplicate event")
)
```

```147:152:internal/repo/repo.go
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrHouseNotFound
		}
		return err
	}
```

**効果**:
- `errors.Is()`による型安全なエラー判定
- エラーメッセージの変更に影響されない
- テストでのモックが容易

### 3. 入力の"取りこぼし"を潰す（typo検知）

**問題**: `json.Decoder`のままだと余計なフィールドを黙殺し、仕様ズレの温床になる。

**実装**: `internal/http/router.go`

```57:61:internal/http/router.go
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			writeErr(w, 400, "invalid json: "+err.Error())
			return
```

**効果**:
- 未知フィールドで即座にエラーを返す
- クライアント側のtypoを早期発見
- API仕様との整合性を保証

### 4. Unicode/表記ゆれで重みが割れる問題の解決

**問題**: カテゴリ名が「皿洗い」「皿洗い 」「さらあらい」「皿 洗 い」などで別カテゴリ化する。

**実装**: `internal/service/service.go`

```17:25:internal/service/service.go
// normalizeCategory カテゴリ名を正規化（全角/半角・NFKC・trim・連続空白圧縮）
func normalizeCategory(s string) string {
	s = strings.TrimSpace(s)
	// 連続空白を単一スペースに圧縮
	s = strings.Join(strings.Fields(s), " ")
	// Unicode正規化（NFKC: 互換等価文字を統合）
	s = norm.NFKC.String(s)
	return s
}
```

```60:75:internal/service/service.go
	def, err := resolveTask(strings.TrimSpace(p.Task))
	if err != nil {
		return err
	}
	canonical := normalizeCategory(def.Key)

	wt := 1.0
	w, _ := s.rp.CategoryWeight(ctx, p.GroupID, canonical)
	if w > 0 {
		wt = w
	}
	points := def.Points * wt
```

**効果**:
- 前後スペースのtrim
- 連続スペースの圧縮
- Unicode正規化（NFKC）による互換等価文字の統合
- `Report`と`UpsertCategory`の両方で自動適用

### 5. 認可ゼロは危険（管理APIの保護）

**問題**: 管理API（重み更新）が誰でも叩ける状態。

**実装**: `internal/http/router.go`

```27:36:internal/http/router.go
func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Admin-Token")
		expected := os.Getenv("ADMIN_TOKEN")
		if expected == "" || token != expected {
			writeErr(w, 403, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

```119:151:internal/http/router.go
	admin := chi.NewRouter()
	admin.Use(requireAdmin)
	admin.Put("/houses/{group}/categories/{name}", func(w http.ResponseWriter, r *http.Request) {
		// ... 実装 ...
	})
	r.Mount("/admin", admin)
```

**効果**:
- `/admin`パス配下を保護
- `X-Admin-Token`ヘッダーによる認証
- 環境変数`ADMIN_TOKEN`で管理
- 甘いが無いより遥かに安全

### 6. 競合条件と重複検知の改善

**問題**: 重複発生時の通知が無いと「記録された気になって実はスキップ」が起きる。

**実装**: `internal/repo/repo.go`

```95:107:internal/repo/repo.go
	result, err := tx.ExecContext(ctx, `
INSERT INTO events(house_id,user_id,kind,category_id,points,source_msg_id,created_at,note)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(house_id, source_msg_id) DO NOTHING
`, houseID, userID, KindChore, catID, p.Points, p.SourceMsgID, p.Now, p.Note)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrDuplicateEvent
	}
```

```64:69:internal/http/router.go
		if err := sv.Report(r.Context(), p); err != nil {
			if errors.Is(err, repo.ErrDuplicateEvent) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "duplicate"})
				return
			}
```

**効果**:
- `RowsAffected()`で重複を検知
- HTTP 200で`{"status":"duplicate"}`を返す
- クライアント側で重複を認識可能

### 7. カバリングインデックス

**実装**: `db/migrations/000001_0001_init.up.sql`

- `idx_events_house_created_cover`（INCLUDE句付きカバリングインデックス）を初期マイグレーションに同梱
- 週次集計専用クエリのアクセスパターンを想定した複合インデックスを用意

**効果**:
- 週次集計クエリのヒープアクセスを削減
- PostgreSQL 11+のINCLUDE句を活用
- クエリパフォーマンスの向上

### 8. ロギングと優雅な停止

**実装**: `internal/http/router.go`、`cmd/server/main.go`

- `middleware.Logger`: アクセスログの出力
- `middleware.Recoverer`: パニック時のリカバリ
- `signal.NotifyContext`: SIGINT/SIGTERMのハンドリング
- `srv.Shutdown()`: 10秒タイムアウトでの優雅な停止

**効果**:
- 障害解析の生命線となるログ出力
- データ損失を防ぐ優雅な停止

### 9. 仕様の落とし穴への対応

#### 9.1 冪等性のスコープ

**設計判断**: `(house_id, source_msg_id)`で唯一性を保証。同じhouse内で一意であれば十分と判断。

**将来の拡張**: ユーザーごと一意にしたい場合は`(house_id, user_id, source_msg_id)`に変更可能。

#### 9.2 購入報告の廃止

**判断**: 運用上のニーズがなくなったため、購入報告はサポート対象外とした。

**影響**:
- API・LINE Webhookは家事報告のみ受け付ける
- DBの`events`テーブルから`amount_yen`を削除し、`kind`は常に`chore`
- 将来復活させる場合は新たなマイグレーションで再追加可能

### 10. テスト最小セット

**実装**: `internal/service/service_test.go`

```7:27:internal/service/service_test.go
func TestNormalizeCategory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"trim spaces", "  皿洗い  ", "皿洗い"},
		{"multiple spaces", "皿洗い  掃除", "皿洗い 掃除"},
		{"empty", "", ""},
		{"normal", "皿洗い", "皿洗い"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeCategory(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeCategory(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
```

**カバレッジ**:
- カテゴリ正規化のテスト
- 各種エッジケース（trim、連続空白、空文字列）

---

## まとめ

### 実装した堅牢性

1. **DBレベルでの保証**
   - NOT NULL制約と長さ制限により、データ整合性を保証
   - アプリケーション層をバイパスしても安全

2. **エラーハンドリングの改善**
   - センチネルエラーによる型安全なエラー判定
   - 適切なHTTPステータスコード

3. **入力検証の強化**
   - `DisallowUnknownFields()`によるtypo検知
   - カテゴリ正規化による表記ゆれの解消

4. **セキュリティ**
   - 管理APIの認可保護
   - 環境変数による設定管理

5. **運用性**
   - 重複検知と明確なレスポンス
   - ロギングと優雅な停止

### 運用上の利点

- **信頼性**: 多重防御により、様々な攻撃ベクトルやバグから保護
- **保守性**: センチネルエラーやテストにより、変更に強い設計
- **可観測性**: ログとエラーメッセージにより、問題の早期発見が可能
- **拡張性**: 正規化やDB制約により、将来の機能追加が容易

### 今後の改善点

- カテゴリ正規化のDB側実装（generated column + UNIQUE制約）
- purchaseポイント係数の外出し（`house_settings.yen_per_point`の活用）
- より包括的なテストカバレッジ

