package repo

import (
    "database/sql"
    "errors"
    "regexp"
    "testing"
    "time"

    sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestDeleteLatestEventSuccess(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New failed: %v", err)
    }
    defer db.Close()

    r := New(db)
    now := time.Now()

    mock.ExpectBegin()
    mock.ExpectQuery(regexp.QuoteMeta(`WITH target AS (
    SELECT e.id, e.task_key, e.points, e.created_at
    FROM events e
    JOIN houses h ON h.id = e.house_id
    JOIN users u  ON u.id = e.user_id
    WHERE h.ext_group_id = $1 AND u.ext_user_id = $2
    ORDER BY e.created_at DESC
    LIMIT 1
)
SELECT t.id, t.task_key, t.points, t.created_at FROM target t
`)).
        WithArgs("g1", "u1").
        WillReturnRows(sqlmock.NewRows([]string{"id", "task_key", "points", "created_at"}).
            AddRow(42, "皿洗い", 150.0, now))
    mock.ExpectExec(regexp.QuoteMeta("DELETE FROM events WHERE id=$1")).
        WithArgs(int64(42)).
        WillReturnResult(sqlmock.NewResult(0, 1))
    mock.ExpectCommit()

    out, err := r.DeleteLatestEvent(nil, "g1", "u1")
    if err != nil {
        t.Fatalf("DeleteLatestEvent returned error: %v", err)
    }
    if out.TaskKey != "皿洗い" || out.Points != 150.0 {
        t.Fatalf("unexpected result: %+v", out)
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("expectations not met: %v", err)
    }
}

func TestDeleteLatestEventNoRows(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New failed: %v", err)
    }
    defer db.Close()

    r := New(db)

    mock.ExpectBegin()
    mock.ExpectQuery(regexp.QuoteMeta(`WITH target AS (
    SELECT e.id, e.task_key, e.points, e.created_at
    FROM events e
    JOIN houses h ON h.id = e.house_id
    JOIN users u  ON u.id = e.user_id
    WHERE h.ext_group_id = $1 AND u.ext_user_id = $2
    ORDER BY e.created_at DESC
    LIMIT 1
)
SELECT t.id, t.task_key, t.points, t.created_at FROM target t
`)).
        WithArgs("g1", "u1").
        WillReturnError(sql.ErrNoRows)
    mock.ExpectRollback()

    _, err = r.DeleteLatestEvent(nil, "g1", "u1")
    if !errors.Is(err, ErrNoEventFound) {
        t.Fatalf("expected ErrNoEventFound, got %v", err)
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("expectations not met: %v", err)
    }
}

func TestDeleteLatestEventDeleteFailure(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New failed: %v", err)
    }
    defer db.Close()

    r := New(db)
    now := time.Now()

    mock.ExpectBegin()
    mock.ExpectQuery(regexp.QuoteMeta(`WITH target AS (
    SELECT e.id, e.task_key, e.points, e.created_at
    FROM events e
    JOIN houses h ON h.id = e.house_id
    JOIN users u  ON u.id = e.user_id
    WHERE h.ext_group_id = $1 AND u.ext_user_id = $2
    ORDER BY e.created_at DESC
    LIMIT 1
)
SELECT t.id, t.task_key, t.points, t.created_at FROM target t
`)).
        WithArgs("g1", "u1").
        WillReturnRows(sqlmock.NewRows([]string{"id", "task_key", "points", "created_at"}).AddRow(42, "皿洗い", 150.0, now))
    mock.ExpectExec(regexp.QuoteMeta("DELETE FROM events WHERE id=$1")).
        WithArgs(int64(42)).
        WillReturnError(errors.New("delete failed"))
    mock.ExpectRollback()

    _, err = r.DeleteLatestEvent(nil, "g1", "u1")
    if err == nil || err.Error() != "delete failed" {
        t.Fatalf("expected delete failed error, got %v", err)
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("expectations not met: %v", err)
    }
}

