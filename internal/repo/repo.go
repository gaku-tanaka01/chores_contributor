package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ db *pgxpool.Pool }
func New(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

func (r *Repo) Ping(ctx context.Context) error {
	return r.db.Ping(ctx)
}

var (
	ErrHouseNotFound   = errors.New("house not found")
	ErrDuplicateEvent  = errors.New("duplicate event")
)

type EventKind string
const (
	KindChore    EventKind = "chore"
	KindPurchase EventKind = "purchase"
)

type UpsertHouseUserParams struct {
	ExtGroupID string
	ExtUserID  string
	DisplayName *string
}

type InsertEventParams struct {
	ExtGroupID  string
	ExtUserID   string
	Kind        EventKind
	Category    *string
	Minutes     *int
	AmountYen   *int
	Points      float64
	SourceMsgID *string
	Now         time.Time
	Note        *string
}

func (r *Repo) InsertEvent(ctx context.Context, p InsertEventParams) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil { return err }
	defer func() { _ = tx.Rollback(ctx) }()

	var houseID int64
	err = tx.QueryRow(ctx, `
INSERT INTO houses(ext_group_id) VALUES($1)
ON CONFLICT(ext_group_id) DO UPDATE SET name=COALESCE(houses.name, EXCLUDED.ext_group_id)
RETURNING id
`, p.ExtGroupID).Scan(&houseID)
	if err != nil { return err }

	var userID int64
	err = tx.QueryRow(ctx, `
INSERT INTO users(ext_user_id) VALUES($1)
ON CONFLICT(ext_user_id) DO UPDATE SET display_name=COALESCE(users.display_name, NULL)
RETURNING id
`, p.ExtUserID).Scan(&userID)
	if err != nil { return err }

	_, err = tx.Exec(ctx, `
INSERT INTO memberships(house_id,user_id) VALUES($1,$2)
ON CONFLICT(house_id,user_id) DO NOTHING
`, houseID, userID)
	if err != nil { return err }

	var catID *int64
	if p.Category != nil {
		var id int64
		err = tx.QueryRow(ctx, `
INSERT INTO categories(house_id,name,weight)
VALUES($1,$2,1.0)
ON CONFLICT(house_id,name) DO UPDATE SET name=EXCLUDED.name
RETURNING id
`, houseID, *p.Category).Scan(&id)
		if err != nil { return err }
		catID = &id
	}

	ct, err := tx.Exec(ctx, `
INSERT INTO events(house_id,user_id,kind,category_id,minutes,amount_yen,points,source_msg_id,created_at,note)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT(house_id, source_msg_id) DO NOTHING
`, houseID, userID, p.Kind, catID, p.Minutes, p.AmountYen, p.Points, p.SourceMsgID, p.Now, p.Note)
	if err != nil { return err }
	if ct.RowsAffected() == 0 {
		return ErrDuplicateEvent
	}

	return tx.Commit(ctx)
}

type WeeklyRow struct {
	Name   string  `json:"name"`
	Points float64 `json:"points"`
}

func (r *Repo) WeeklyPoints(ctx context.Context, extGroupID string, start, end time.Time) ([]WeeklyRow, error) {
	rows, err := r.db.Query(ctx, `
SELECT COALESCE(u.display_name, substr(u.ext_user_id,1,6)) AS name,
       COALESCE(SUM(e.points),0) AS pt
FROM events e
JOIN users u  ON u.id=e.user_id
JOIN houses h ON h.id=e.house_id
WHERE h.ext_group_id=$1 AND e.created_at >= $2 AND e.created_at < $3
GROUP BY u.id, u.display_name, u.ext_user_id
ORDER BY pt DESC
`, extGroupID, start, end)
	if err != nil { return nil, err }
	defer rows.Close()

	var out []WeeklyRow
	for rows.Next() {
		var w WeeklyRow
		if err := rows.Scan(&w.Name, &w.Points); err != nil { return nil, err }
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *Repo) CategoryWeight(ctx context.Context, extGroupID, category string) (float64, error) {
	var weight float64 = 1.0
	err := r.db.QueryRow(ctx, `
SELECT c.weight
FROM categories c
JOIN houses h ON h.id=c.house_id
WHERE h.ext_group_id=$1 AND c.name=$2
`, extGroupID, category).Scan(&weight)
	if err != nil {
		// 未定義なら1.0で進む
		return 1.0, nil
	}
	return weight, nil
}

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
