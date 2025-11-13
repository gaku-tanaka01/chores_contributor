# ==== common ====
SHELL := /usr/bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.ONESHELL:
.DEFAULT_GOAL := help

# ---- optional: local .env ----
ifneq ("$(wildcard .env)","")
include .env
export $(shell sed -ne 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' .env)
endif

# ==== defaults (local) ====
POSTGRES_USER      ?= app
POSTGRES_PASSWORD  ?= app
POSTGRES_HOST      ?= localhost
POSTGRES_PORT      ?= 5432
POSTGRES_DB        ?= chores
SSL_MODE           ?= disable
PORT               ?= 8081
N                  ?= 1

DATABASE_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=$(SSL_MODE)
PGURL        ?= $(DATABASE_URL)

# ==== paths ====
MIGRATIONS_DIR ?= db/migrations
BACKUP_DIR     ?= ./backup

# ==== commands ====
MIGRATE = migrate -database "$(DATABASE_URL)" -path "$(MIGRATIONS_DIR)"
PSQL    = psql "$(DATABASE_URL)"

# ==== help ====
.PHONY: help
help:
	@echo "Make targets:"
	@echo "  migrate-up                # すべて適用（ローカル）"
	@echo "  migrate-down [N=1]        # N 本ロールバック"
	@echo "  migrate-goto V=3          # バージョンへ移動"
	@echo "  migrate-force V=3         # version 強制設定"
	@echo "  migrate-drop              # 全削除(確認あり)"
	@echo "  migrate-version           # 現在のバージョン"
	@echo "  migrate-create name=x     # 新規ファイル作成"
	@echo "  backup                    # ローカルdump (--no-owner 等付き)"
	@echo "  db-truncate               # 主要テーブルTRUNCATE"
	@echo "  db-wait                   # DB起動待ち"
	@echo "  migrate-up-prod           # 本番(.env.local.production読込) 適用"
	@echo "  migrate-down-prod [N=1]   # 本番ロールバック"
	@echo "  backup-prod               # 本番dump"
	@echo ""
	@echo "env: DATABASE_URL=$(DATABASE_URL)"
	@echo "migrations: $(MIGRATIONS_DIR)"
	@echo "port: $(PORT)"

# ==== local tasks ====
.PHONY: migrate-up
migrate-up: db-wait
	@$(MIGRATE) up

.PHONY: migrate-down
migrate-down: db-wait
	@$(MIGRATE) down $(N)

.PHONY: migrate-goto
migrate-goto: db-wait
	@test -n "$(V)" || { echo "V=version を指定"; exit 2; }
	@$(MIGRATE) goto $(V)

.PHONY: migrate-force
migrate-force: db-wait
	@test -n "$(V)" || { echo "V=version を指定"; exit 2; }
	@$(MIGRATE) force $(V)

.PHONY: migrate-drop
migrate-drop: db-wait
	@read -p "!!! DROP ALL TABLES. continue? [y/N] " ans; \
	[[ "$$ans" =~ ^[yY]$$ ]] && $(MIGRATE) drop -f || echo "aborted"

.PHONY: migrate-version
migrate-version: db-wait
	@$(MIGRATE) version

.PHONY: migrate-create
migrate-create:
	@test -n "$(name)" || { echo "name=xxx を指定"; exit 2; }
	@mkdir -p "$(MIGRATIONS_DIR)"
	@migrate create -dir "$(MIGRATIONS_DIR)" -ext sql -seq "$(name)"

.PHONY: backup
backup:
	@mkdir -p $(BACKUP_DIR)
	@pg_dump $(PGURL) -Fc --no-owner --no-privileges -f $(BACKUP_DIR)/$$(date +%Y%m%d-%H%M).dump
	@echo "backup created: $(BACKUP_DIR)/$$(date +%Y%m%d-%H%M).dump"

.PHONY: sql
sql: db-wait
	@$(PSQL)

.PHONY: db-wait
db-wait:
	@echo "waiting for postgres at $(POSTGRES_HOST):$(POSTGRES_PORT)..."
	@if command -v pg_isready >/dev/null 2>&1; then
		for i in $$(seq 1 30); do
			pg_isready -h $(POSTGRES_HOST) -p $(POSTGRES_PORT) >/dev/null 2>&1 && { echo "ready"; exit 0; }
			sleep 1
		done
	else
		for i in $$(seq 1 30); do
			psql "$(DATABASE_URL)" -c '\q' >/dev/null 2>&1 && { echo "ready"; exit 0; }
			sleep 1
		done
	fi
	@echo "postgres not ready"; exit 1

.PHONY: db-truncate
db-truncate: db-wait
	@read -p "truncate tables? [y/N] " ans; \
	[[ "$$ans" =~ ^[yY]$$ ]] && \
	$(PSQL) -v ON_ERROR_STOP=1 -c "\
		TRUNCATE TABLE \
			events, \
			memberships, \
			houses, \
			users \
		RESTART IDENTITY CASCADE;" \
	|| echo "aborted"

# ==== prod helpers (explicit .env.local.production load) ====
define LOAD_PROD
set -euo pipefail
set -a
source .env.local.production
set +a
endef

# prod MIGRATE alias (uses env from .env.local.production)
define RUN_MIGRATE_PROD
$(LOAD_PROD)
migrate -database "$${DATABASE_URL}" -path "$${MIGRATIONS_DIR:-$(MIGRATIONS_DIR)}"
endef

.PHONY: migrate-up-prod
migrate-up-prod:
	@$(LOAD_PROD); \
	read -p "Apply migrations to PRODUCTION? [y/N] " ans; \
	if [[ "$$ans" =~ ^[yY]$$ ]]; then \
		$(RUN_MIGRATE_PROD) up; \
	else echo "aborted"; fi

.PHONY: migrate-down-prod
migrate-down-prod:
	@$(LOAD_PROD); \
	read -p "Rollback PRODUCTION N=$(N)? [y/N] " ans; \
	if [[ "$$ans" =~ ^[yY]$$ ]]; then \
		$(RUN_MIGRATE_PROD) down $(N); \
	else echo "aborted"; fi

.PHONY: backup-prod
backup-prod:
	@$(LOAD_PROD); \
	mkdir -p $(BACKUP_DIR); \
	pg_dump "$${DATABASE_URL}" -Fc --no-owner --no-privileges -f $(BACKUP_DIR)/$$(date +%Y%m%d-%H%M)-prod.dump; \
	echo "backup created: $(BACKUP_DIR)/$$(date +%Y%m%d-%H%M)-prod.dump"
