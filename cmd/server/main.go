package main

// .env.local.production

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"chores_contributor/internal/db"
	httpapi "chores_contributor/internal/http"
	"chores_contributor/internal/repo"
	"chores_contributor/internal/service"
)

func init() {
	var envPath string
	if os.Getenv("RENDER") != "" {
		// Render本番環境: /etc/secrets/.env.local
		envPath = "/etc/secrets/.env.production.local"
	} else {
		// ローカル開発環境: プロジェクトルートの.env.local
		envPath = ".env.local"
	}
	
	err := godotenv.Load(envPath)
	if err != nil {
		log.Fatalf("Error loading .env file from %s: %v", envPath, err)
	}
}

func getenv(k, defaultValue string) string {
	v := os.Getenv(k)
	if v == "" {
		if defaultValue == "" {
			log.Fatalf("%s is required", k)
		}
		return defaultValue
	}
	return v
}

func getPort() string {
	port := getenv("PORT", "8081")
	if _, err := strconv.Atoi(port); err != nil {
		log.Fatalf("PORT must be a number: %s", port)
	}
	return port
}

func main() {
	os.Setenv("TZ", "Asia/Tokyo")

	dsn := getenv("DATABASE_URL", "")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := db.Connect(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	rp := repo.New(pool)
	sv := service.New(rp)

	r := httpapi.Router(sv)

	addr := ":" + getPort()
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shCtx)
}
