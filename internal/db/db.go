package db

import (
	"context"
	"net/url"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(dsn string) (*pgxpool.Pool, error) {
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
		return nil, err
	}
	return pgxpool.NewWithConfig(context.Background(), cfg)
}
