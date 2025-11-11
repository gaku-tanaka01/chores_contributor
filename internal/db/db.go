package db

import (
	"context"
	"net/url"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(dsn string) (*pgxpool.Pool, error) {
	// sslmode=require を強制（Render/Supabase用）
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if q.Get("sslmode") == "" {
		q.Set("sslmode", "require")
		u.RawQuery = q.Encode()
		dsn = u.String()
	} else if !strings.Contains(strings.ToLower(q.Get("sslmode")), "require") && !strings.Contains(strings.ToLower(q.Get("sslmode")), "verify") {
		// 本番環境では require または verify を強制
		if os.Getenv("ENV") == "production" || os.Getenv("RENDER") != "" {
			q.Set("sslmode", "require")
			u.RawQuery = q.Encode()
			dsn = u.String()
		}
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(context.Background(), cfg)
}
