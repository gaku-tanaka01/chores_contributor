//go:build go1.20

package db

import (
	"context"
	"database/sql"
	"log"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// OpenIPv4DB forces pgx to dial using IPv4 only and waits until the DB responds to Ping.
func OpenIPv4DB(dsn string) *sql.DB {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("pgx.ParseConfig failed: %v", err)
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	cfg.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err == nil && len(ips) > 0 {
			return dialer.DialContext(ctx, "tcp4", net.JoinHostPort(ips[0].String(), port))
		}
		return dialer.DialContext(ctx, "tcp4", addr)
	}

	db := stdlib.OpenDB(*cfg)

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		if err := db.PingContext(ctx); err == nil {
			log.Printf("db ping ok (IPv4 forced)")
			break
		}
		select {
		case <-ctx.Done():
			log.Fatalf("db ping failed (IPv4): %v", ctx.Err())
		default:
			time.Sleep(1 * time.Second)
		}
	}
	return db
}
