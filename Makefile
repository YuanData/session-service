## Phase 1 – 基本環境與 API

.PHONY: help
help: ## 顯示可用目標
	@echo "可用指令（依情境排序）："
	@echo "  make deps            安裝 / 更新 Go 依賴 (go mod tidy)"
	@echo "  make sqlc            產生 sqlc 程式碼 (sqlc generate)"
	@echo "  make copy-env        從 .env.example 複製 .env"
	@echo "  make run-api         啟動 API 伺服器 (Phase 1/2/3 共用)"
	@echo "  make clean-db        刪除本機 SQLite DB 檔 data/app.db"
	@echo "  make redis-up        以 Docker 啟動 Redis 7.4"
	@echo "  make redis-cli       進入 redis-cli"
	@echo "  make test            執行 go test ./... -cover"
	@echo "  make test-deps       安裝測試相依套件"
	@echo "  make worker          啟動 Asynq worker (Phase 3)"
	@echo "  make phase2-login    Phase 2 範例：Signup + Login + /me"
	@echo "  make phase2-logout   Phase 2 範例：Logout 後 /me 失效"
	@echo "  make phase2-maxsess  Phase 2 範例：同時登入數限制測試"
	@echo "  make admin-sessions  Phase 3 Admin：列出某 user sessions"
	@echo "  make admin-kick-one  Phase 3 Admin：踢掉單一 session"
	@echo "  make admin-kick-all  Phase 3 Admin：踢掉所有 sessions"
	@echo "  make admin-ban       Phase 3 Admin：封鎖 user"
	@echo "  make admin-unban     Phase 3 Admin：解封 user"

## 共用變數

BASE_URL ?= http://localhost:8080
ADMIN_TOKEN ?= dev-admin

## Phase 1 – 啟動前準備與啟動 API

.PHONY: deps
deps: ## go mod tidy
	go mod tidy

.PHONY: sqlc
sqlc: ## sqlc generate
	sqlc generate

.PHONY: copy-env
copy-env: ## cp .env.example .env
	cp .env.example .env

.PHONY: run-api
run-api: ## go run ./cmd/api
	go run ./cmd/api

.PHONY: clean-db
clean-db: ## rm -f ./data/app.db
	rm -f ./data/app.db

## Phase 2 – Redis、Session 管理與測試腳本

.PHONY: redis-up
redis-up: ## docker run --rm -p 6379:6379 redis:7.4-alpine
	docker run --rm -p 6379:6379 redis:7.4-alpine

.PHONY: redis-cli
redis-cli: ## redis-cli
	redis-cli

.PHONY: phase2-login
phase2-login: ## README Phase 2 範例：Signup + Login + /me
	@BASE_URL="$(BASE_URL)"; \
	echo "BASE_URL=$$BASE_URL"; \
	echo "== Signup =="; \
	curl -s -X POST "$$BASE_URL/auth/signup" \
	  -H "Content-Type: application/json" \
	  -d '{"username":"alice","password":"password123"}'; \
	echo ""; \
	echo "== Login =="; \
	LOGIN_RES=$$(curl -s -X POST "$$BASE_URL/auth/login" \
	  -H "Content-Type: application/json" \
	  -d '{"username":"alice","password":"password123"}'); \
	echo "$$LOGIN_RES"; \
	TOKEN=$$(echo "$$LOGIN_RES" | jq -r '.access_token'); \
	echo "TOKEN=$$TOKEN"; \
	echo "== /me =="; \
	curl -s "$$BASE_URL/me" \
	  -H "Authorization: Bearer $$TOKEN"; \
	echo ""

.PHONY: phase2-logout
phase2-logout: ## README Phase 2 範例：Logout 後 /me 失效（需先有 TOKEN 環境變數）
	@[ -n "$$TOKEN" ] || (echo "請先設定環境變數 TOKEN（或先執行 make phase2-login）"; exit 1)
	@BASE_URL="$(BASE_URL)"; \
	echo "使用 TOKEN=$$TOKEN 呼叫 /auth/logout 與 /me"; \
	echo "== /auth/logout =="; \
	curl -s -X POST "$$BASE_URL/auth/logout" \
	  -H "Authorization: Bearer $$TOKEN"; \
	echo ""; \
	echo "== /me (預期 401) =="; \
	curl -i "$$BASE_URL/me" \
	  -H "Authorization: Bearer $$TOKEN" || true; \
	echo ""

.PHONY: phase2-maxsess
phase2-maxsess: ## README Phase 2 範例：同時登入數限制測試
	@BASE_URL="$(BASE_URL)"; \
	echo "BASE_URL=$$BASE_URL"; \
	echo "連續登入三次以測試 MaxSessionsPerUser"; \
	TOKENS=(); \
	for i in 1 2 3; do \
	  echo "== Login $$i =="; \
	  RES=$$(curl -s -X POST "$$BASE_URL/auth/login" \
	    -H "Content-Type: application/json" \
	    -d '{"username":"alice","password":"password123"}'); \
	  echo "$$RES"; \
	  TOKENS+=($$(echo "$$RES" | jq -r '.access_token')); \
	done; \
	TOKEN1=$${TOKENS[0]}; \
	TOKEN2=$${TOKENS[1]}; \
	TOKEN3=$${TOKENS[2]}; \
	echo "T1=$$TOKEN1"; \
	echo "T2=$$TOKEN2"; \
	echo "T3=$$TOKEN3"; \
	echo "== 使用三個 token 呼叫 /me，預期 T1 401, T2/T3 200 =="; \
	echo "-- /me with TOKEN1 (oldest) --"; \
	curl -i "$$BASE_URL/me" -H "Authorization: Bearer $$TOKEN1" || true; \
	echo ""; \
	echo "-- /me with TOKEN2 --"; \
	curl -i "$$BASE_URL/me" -H "Authorization: Bearer $$TOKEN2" || true; \
	echo ""; \
	echo "-- /me with TOKEN3 --"; \
	curl -i "$$BASE_URL/me" -H "Authorization: Bearer $$TOKEN3" || true; \
	echo ""

## Phase 3 – Asynq Worker 與 Admin API

.PHONY: worker
worker: ## go run ./cmd/worker
	go run ./cmd/worker

.PHONY: admin-sessions
admin-sessions: ## GET /admin/users/1/sessions
	@BASE_URL="$(BASE_URL)"; \
	ADMIN_TOKEN="$(ADMIN_TOKEN)"; \
	echo "列出 user 1 的活躍 sessions"; \
	curl -s "$$BASE_URL/admin/users/1/sessions" \
	  -H "X-Admin-Token: $$ADMIN_TOKEN"; \
	echo ""

.PHONY: admin-kick-one
admin-kick-one: ## POST /admin/users/1/kick 單一 session，需要 SESSION_ID 變數
	@[ -n "$$SESSION_ID" ] || (echo "請先設定環境變數 SESSION_ID"; exit 1)
	@BASE_URL="$(BASE_URL)"; \
	ADMIN_TOKEN="$(ADMIN_TOKEN)"; \
	echo "踢掉 user 1 的單一 session: $$SESSION_ID"; \
	curl -s -X POST "$$BASE_URL/admin/users/1/kick" \
	  -H "Content-Type: application/json" \
	  -H "X-Admin-Token: $$ADMIN_TOKEN" \
	  -d "{\"session_id\":\"$$SESSION_ID\"}"; \
	echo ""

.PHONY: admin-kick-all
admin-kick-all: ## POST /admin/users/1/kick all=true
	@BASE_URL="$(BASE_URL)"; \
	ADMIN_TOKEN="$(ADMIN_TOKEN)"; \
	echo "踢掉 user 1 的所有 sessions"; \
	curl -s -X POST "$$BASE_URL/admin/users/1/kick" \
	  -H "Content-Type: application/json" \
	  -H "X-Admin-Token: $$ADMIN_TOKEN" \
	  -d '{"all":true}'; \
	echo ""

.PHONY: admin-ban
admin-ban: ## POST /admin/users/1/ban
	@BASE_URL="$(BASE_URL)"; \
	ADMIN_TOKEN="$(ADMIN_TOKEN)"; \
	echo "封鎖 user 1"; \
	curl -s -X POST "$$BASE_URL/admin/users/1/ban" \
	  -H "X-Admin-Token: $$ADMIN_TOKEN"; \
	echo ""

.PHONY: admin-unban
admin-unban: ## POST /admin/users/1/unban
	@BASE_URL="$(BASE_URL)"; \
	ADMIN_TOKEN="$(ADMIN_TOKEN)"; \
	echo "解封 user 1"; \
	curl -s -X POST "$$BASE_URL/admin/users/1/unban" \
	  -H "X-Admin-Token: $$ADMIN_TOKEN"; \
	echo ""

## 測試相關

.PHONY: test-deps
test-deps: ## 安裝測試相依：testify 與 miniredis
	go get github.com/stretchr/testify
	go get github.com/alicebob/miniredis/v2

.PHONY: test
test: ## go test ./... -cover
	go test ./... -cover


