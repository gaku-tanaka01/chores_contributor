# Makefile — migrate 操作用

# ==== .env を読み込む（存在すれば） ====
ifneq ("$(wildcard .env)","")
include .env
# .env のキーを export（空白・引用符のない KEY=VAL 前提）
export $(shell sed -ne 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' .env)
endif

# ==== 接続設定（.env 未設定ならデフォルト） ====
POSTGRES_USER      ?= app
POSTGRES_PASSWORD  ?= app
POSTGRES_HOST      ?= localhost
POSTGRES_PORT      ?= 5432
POSTGRES_DB        ?= chores
SSL_MODE           ?= disable
PORT               ?= 8081

# DATABASE_URL が無ければ組み立てる
DATABASE_URL       ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=$(SSL_MODE)
PGURL              ?= $(DATABASE_URL)

# ==== パス ====
MIGRATIONS_DIR     ?= db/migrations
BACKUP_DIR         ?= ./backup

# ==== コマンド ====
MIGRATE            ?= migrate -database "$(DATABASE_URL)" -path "$(MIGRATIONS_DIR)"
PSQL               ?= psql "$(DATABASE_URL)"

# ==== ヘルパ ====
.DEFAULT_GOAL := help

.PHONY: help
help: ## このヘルプ
	@echo "Make targets:"
	@echo "  make migrate-up            # すべて適用"
	@echo "  make migrate-down [N=1]    # N 本ロールバック（デフォルト1）"
	@echo "  make migrate-goto V=3      # バージョン V へ移動"
	@echo "  make migrate-force V=3     # DBのバージョンを強制設定（失敗時の手当）"
	@echo "  make migrate-drop          # すべてのテーブル削除（要確認）"
	@echo "  make migrate-version       # 現在のバージョン表示"
	@echo "  make migrate-create name=x # 新規マイグレーション作成（seq, .sql）"
	@echo "  make db-wait               # DB起動待ち（ヘルス待機）"
	@echo "  make backup                # DBバックアップ作成"
	@echo "  make db-truncate           # 主要テーブルをTRUNCATE（RESTART IDENTITY）"
	@echo "  make migrate-up-safe       # バックアップ後にマイグレーション実行"
	@echo "  make post-chore            # 家事報告のテスト送信"
	@echo "  make weekly               # 週次集計の取得"
	@echo ""
	@echo "env: DATABASE_URL=$(DATABASE_URL)"
	@echo "migrations: $(MIGRATIONS_DIR)"
	@echo "port: $(PORT)"

# ==== タスク ====
.PHONY: backup
backup:
	@mkdir -p $(BACKUP_DIR)
	@pg_dump $(PGURL) -Fc -f $(BACKUP_DIR)/$$(date +%Y%m%d-%H%M).dump
	@echo "backup created: $(BACKUP_DIR)/$$(date +%Y%m%d-%H%M).dump"

.PHONY: migrate-up
migrate-up: db-wait
	@$(MIGRATE) up

.PHONY: migrate-up-safe
migrate-up-safe: backup migrate-up

.PHONY: migrate-down
migrate-down: db-wait
	@$(MIGRATE) down $(N)

.PHONY: migrate-goto
migrate-goto: db-wait
	@if [ -z "$(V)" ]; then echo "V=version を指定しろ"; exit 2; fi
	@$(MIGRATE) goto $(V)

.PHONY: migrate-force
migrate-force: db-wait
	@if [ -z "$(V)" ]; then echo "V=version を指定しろ"; exit 2; fi
	@$(MIGRATE) force $(V)

.PHONY: migrate-drop
migrate-drop: db-wait
	@read -p "!!! ALL TABLES WILL BE DROPPED. continue? [y/N] " ans; \
	if [ "$$ans" = "y" ] || [ "$$ans" = "Y" ]; then \
		$(MIGRATE) drop -f; \
	else \
		echo "aborted"; \
	fi

.PHONY: migrate-version
migrate-version: db-wait
	@$(MIGRATE) version

.PHONY: migrate-create
migrate-create:
	@if [ -z "$(name)" ]; then echo "name=xxx を指定しろ"; exit 2; fi
	@mkdir -p "$(MIGRATIONS_DIR)"
	@migrate create -dir "$(MIGRATIONS_DIR)" -ext sql -seq "$(name)"

# psql ショートカット（任意）
.PHONY: sql
sql: db-wait
	@$(PSQL)

# DBが受け付け可能になるまで待機
.PHONY: db-wait
db-wait:
	@echo "waiting for postgres at $(POSTGRES_HOST):$(POSTGRES_PORT)..."
	@for i in $$(seq 1 30); do \
		echo "\q" | $(PSQL) >/dev/null 2>&1 && { echo "postgres is ready"; exit 0; }; \
		sleep 1; \
	done; \
	echo "postgres not ready"; exit 1

.PHONY: db-truncate
db-truncate: db-wait
	@read -p "truncate tables? this cannot be undone. continue? [y/N] " ans; \
	if [ "$$ans" = "y" ] || [ "$$ans" = "Y" ]; then \
		$(PSQL) -v ON_ERROR_STOP=1 -c '\
			TRUNCATE TABLE \
				events, \
				memberships, \
				houses, \
				users \
			RESTART IDENTITY CASCADE;'; \
		echo "tables truncated."; \
	else \
		echo "aborted"; \
	fi

# ==== curl タスク（APIテスト用） ====
.PHONY: post-chore weekly
post-chore:
	@curl -sS -i -X POST http://localhost:$(PORT)/events/report \
	  -H "Content-Type: application/json" \
	  -d '{"group_id":"default-house","user_id":"u1","task":"皿洗い","source_msg_id":"$${ID:-test-$$RANDOM}"}' | sed -n '1p'

weekly:
	@curl -sS "http://localhost:$(PORT)/houses/default-house/weekly" | jq .
