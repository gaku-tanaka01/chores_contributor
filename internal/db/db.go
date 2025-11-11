package db

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MaskDSN は DSN の password 部分を伏せて表示する
func MaskDSN(dsn string) string {
	if i := strings.Index(dsn, "://"); i != -1 {
		scheme := dsn[:i+3]
		rest := dsn[i+3:]
		if at := strings.Index(rest, "@"); at != -1 {
			cred := rest[:at]
			host := rest[at:]
			if c := strings.SplitN(cred, ":", 2); len(c) == 2 {
				cred = c[0] + ":*****"
			}
			return scheme + cred + host
		}
	}
	return dsn
}

func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	// sslmode=require を強制（Supabase/Render用）
	// SupabaseではSSL接続が必須のため、未指定またはdisableの場合はrequireに上書き
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	currentMode := strings.ToLower(q.Get("sslmode"))

	// Render環境または本番環境では常にrequireを強制
	// ローカル開発環境でもSupabaseを使う場合はrequireが必要
	if os.Getenv("RENDER") != "" || os.Getenv("ENV") == "production" {
		q.Set("sslmode", "require")
	} else if currentMode == "" || currentMode == "disable" || currentMode == "allow" {
		// 未指定または非セキュアな設定の場合はrequireに設定
		q.Set("sslmode", "require")
	}
	// verify-fullやverify-caはそのまま使用（より安全）

	u.RawQuery = q.Encode()
	dsn = u.String()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.ParseConfig: %w", err)
	}

	log.Printf("connecting to database: %s", MaskDSN(dsn))
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.NewWithConfig: %w", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			log.Printf("db ping ok: %s", MaskDSN(dsn))
			return pool, nil
		}
		if time.Now().After(deadline) {
			pool.Close()
			return nil, fmt.Errorf("db ping failed: %w", err)
		}
		log.Printf("db ping retry in 1s: %v", err)
		time.Sleep(1 * time.Second)
	}
}
