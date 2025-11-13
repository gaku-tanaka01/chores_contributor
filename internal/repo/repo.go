package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Repo struct{ db *sql.DB }

func New(db *sql.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

var (
	ErrDuplicateEvent = errors.New("duplicate event")
	ErrNoEventFound   = errors.New("no event found")
)

type EventKind string

const (
	KindChore EventKind = "chore"
)

type UpsertHouseUserParams struct {
	ExtGroupID  string
	ExtUserID   string
	DisplayName *string
}

type InsertEventParams struct {
	ExtGroupID  string
	ExtUserID   string
	DisplayName *string
	TaskKey     string
	TaskOption  *string
	Points      float64
	SourceMsgID *string
	Now         time.Time
	Note        *string
}

func trimmedOrNil(s *string) interface{} {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func (r *Repo) InsertEvent(ctx context.Context, p InsertEventParams) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var houseID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO houses(ext_group_id) VALUES($1)
ON CONFLICT(ext_group_id) DO UPDATE SET name=COALESCE(houses.name, EXCLUDED.ext_group_id)
RETURNING id
`, p.ExtGroupID).Scan(&houseID)
	if err != nil {
		return err
	}

	var userID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO users(ext_user_id, display_name) VALUES($1, $2)
ON CONFLICT(ext_user_id) DO UPDATE SET display_name=COALESCE(EXCLUDED.display_name, users.display_name)
RETURNING id
`, p.ExtUserID, trimmedOrNil(p.DisplayName)).Scan(&userID)
	if err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
INSERT INTO memberships(house_id,user_id) VALUES($1,$2)
ON CONFLICT(house_id,user_id) DO NOTHING
`, houseID, userID); err != nil {
		return err
	}

	result, err := tx.ExecContext(ctx, `
INSERT INTO events(house_id,user_id,kind,task_key,task_option,points,source_msg_id,created_at,note)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT(house_id, source_msg_id) DO NOTHING
`, houseID, userID, KindChore, p.TaskKey, p.TaskOption, p.Points, p.SourceMsgID, p.Now, p.Note)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrDuplicateEvent
	}

	return tx.Commit()
}

func (r *Repo) UpsertHouseUser(ctx context.Context, p UpsertHouseUserParams) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var houseID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO houses(ext_group_id) VALUES($1)
ON CONFLICT(ext_group_id) DO UPDATE SET name=COALESCE(houses.name, EXCLUDED.ext_group_id)
RETURNING id
`, p.ExtGroupID).Scan(&houseID)
	if err != nil {
		return err
	}

	var userID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO users(ext_user_id, display_name) VALUES($1, $2)
ON CONFLICT(ext_user_id) DO UPDATE SET display_name=COALESCE(EXCLUDED.display_name, users.display_name)
RETURNING id
`, p.ExtUserID, trimmedOrNil(p.DisplayName)).Scan(&userID)
	if err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
INSERT INTO memberships(house_id,user_id) VALUES($1,$2)
ON CONFLICT(house_id,user_id) DO NOTHING
`, houseID, userID); err != nil {
		return err
	}

	return tx.Commit()
}

type WeeklyRow struct {
	Name   string  `json:"name"`
	Points float64 `json:"points"`
}

type DeletedEvent struct {
	TaskKey   string
	Points    float64
	CreatedAt time.Time
}

type WeeklyTaskRow struct {
	TaskKey string
	Points  float64
}

func (r *Repo) WeeklyPoints(ctx context.Context, extGroupID string, start, end time.Time) ([]WeeklyRow, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT COALESCE(u.display_name, substr(u.ext_user_id,1,6)) AS name,
       COALESCE(SUM(e.points),0) AS pt
FROM events e
JOIN users u  ON u.id=e.user_id
JOIN houses h ON h.id=e.house_id
WHERE h.ext_group_id=$1 AND e.created_at >= $2 AND e.created_at < $3
GROUP BY u.id, u.display_name, u.ext_user_id
ORDER BY pt DESC
`, extGroupID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WeeklyRow
	for rows.Next() {
		var w WeeklyRow
		if err := rows.Scan(&w.Name, &w.Points); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *Repo) WeeklyUserTaskPoints(ctx context.Context, extGroupID, extUserID string, start, end time.Time) ([]WeeklyTaskRow, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT e.task_key,
       COALESCE(SUM(e.points),0) AS pt
FROM events e
JOIN houses h ON h.id = e.house_id
JOIN users u  ON u.id = e.user_id
WHERE h.ext_group_id = $1
  AND u.ext_user_id = $2
  AND e.created_at >= $3
  AND e.created_at < $4
GROUP BY e.task_key
ORDER BY pt DESC, e.task_key ASC
`, extGroupID, extUserID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WeeklyTaskRow
	for rows.Next() {
		var row WeeklyTaskRow
		if err := rows.Scan(&row.TaskKey, &row.Points); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repo) DeleteLatestEvent(ctx context.Context, extGroupID, extUserID string) (DeletedEvent, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return DeletedEvent{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var result DeletedEvent
	var eventID int64
	err = tx.QueryRowContext(ctx, `
WITH target AS (
    SELECT e.id, e.task_key, e.points, e.created_at
    FROM events e
    JOIN houses h ON h.id = e.house_id
    JOIN users u  ON u.id = e.user_id
    WHERE h.ext_group_id = $1 AND u.ext_user_id = $2
    ORDER BY e.created_at DESC
    LIMIT 1
)
SELECT t.id, t.task_key, t.points, t.created_at FROM target t
`, extGroupID, extUserID).Scan(&eventID, &result.TaskKey, &result.Points, &result.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeletedEvent{}, ErrNoEventFound
		}
		return DeletedEvent{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE id=$1`, eventID); err != nil {
		return DeletedEvent{}, err
	}

	if err := tx.Commit(); err != nil {
		return DeletedEvent{}, err
	}
	return result, nil
}
