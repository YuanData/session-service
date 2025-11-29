## Phase 1 - Minimal Login Service

這是第 1 階段的最小可執行產品，只包含：

- SQLite + sqlc
- Gin HTTP API
- JWT 驗證
- 路由：`/health`、`/auth/signup`、`/auth/login`、`/me`

---

### 目錄結構（Phase 1）

```text
cmd/
  api/
    main.go             # 程式入口：載入設定、開 DB、跑 migration、啟動 Gin

internal/
  config/
    config.go           # 載入 APP_HTTP_ADDR / APP_DB_PATH / APP_JWT_SECRET
  http/
    router.go           # 建立 Gin router，註冊 /health, /auth/*
    handler_auth.go     # signup / login / me 的 handler
  middleware/
    auth_jwt.go         # 解析 Authorization: Bearer，驗證 JWT，塞 userID 到 context
  token/
    jwt.go              # JWT Manager：Generate / Parse，claims 帶 sub(=userID)

db/
  migrations/
    001_init.sql        # users table 建表
  queries/
    users.sql           # CreateUser, GetUserByUsername, GetUserByID（給 sqlc 用）

sqlc.yaml               # sqlc 設定，產出 internal/db package
go.mod                  # Go module & 依賴
```

---

### 啟動前準備

1. 安裝 `sqlc`（若尚未安裝）

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

2. 在專案根目錄執行：

```bash
sqlc generate
```

這會根據：

- `db/migrations/*.sql`
- `db/queries/*.sql`

產生 `internal/db` package，裡面會有：

- `type Queries struct { ... }`
- `func New(db DBTX) *Queries`
- `CreateUser`、`GetUserByUsername`、`GetUserByID` 等方法。

> 若沒有先跑 `sqlc generate`，`go run ./cmd/api` 會因為缺少 `internal/db` 而無法編譯。

---

### 如何在本機啟動 Phase 1

```bash
cd /Users/user/session-service

# 1. 下載依賴
go mod tidy

# 2. 產生 sqlc 程式碼
sqlc generate

# 3. 啟動 API（會自動建立 ./data/app.db 並跑 001_init.sql）
APP_HTTP_ADDR=":8080" \
APP_DB_PATH="./data/app.db" \
APP_JWT_SECRET="dev-secret-change-me" \
go run ./cmd/api
```

啟動後，服務會監聽在 `http://localhost:8080`。

---

### API 一覽（Phase 1）

#### `GET /health`

- 用途：健康檢查
- 回應範例：

```json
{ "status": "ok" }
```

#### `POST /auth/signup`

- Body：

```json
{
  "username": "alice",
  "password": "password123"
}
```

- 行為：
  - 使用 bcrypt 對密碼加鹽雜湊
  - 呼叫 sqlc `CreateUser` 寫入 `users` 表

- 成功回應：

```json
{
  "id": 1,
  "username": "alice"
}
```

#### `POST /auth/login`

- Body：

```json
{
  "username": "alice",
  "password": "password123"
}
```

- 行為：
  - 透過 `GetUserByUsername` 查詢使用者
  - 使用 bcrypt 比對密碼
  - 呼叫 `token.Manager.Generate(userID)` 產生 JWT

- 成功回應：

```json
{
  "access_token": "<JWT>",
  "expires_in": 86400
}
```

#### `GET /me`

- 需要 Header：

```http
Authorization: Bearer <JWT>
```

- 行為：
  - `auth_jwt` middleware 解析 JWT，從 claims 取得 `userID`，放入 Gin context
  - handler 使用 `GetUserByID` 查詢 DB，回傳使用者資訊

- 成功回應：

```json
{
  "id": 1,
  "username": "alice",
  "created": "2025-01-01T00:00:00Z"
}
```

---

### Phase 1 特性與限制

- **沒有 Redis / session 表**：
  - Session 完全存在 JWT 裡，只要 token 沒過期就視為有效。
- **沒有 logout / ban 等功能**：
  - 這些會在 Phase 2、Phase 3 加上（Redis session 管理、Asynq、Admin API）。
- **目的**：
  - 給你一個乾淨、可跑、結構清晰的「最小登入系統」作為之後擴充的基礎。


