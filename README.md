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

---

## Phase 2 - Redis Session 管理

Phase 2 在 Phase 1 的基礎上，加入 **Redis Session 管理 + 同時登入數限制 + /auth/logout**，並讓 `/me` 變成「JWT + Redis 雙重檢查」。

---

### Phase 2 新增/變更重點

- **Config（`internal/config/config.go`）**
  - 新增：
    - `RedisAddr`（預設 `127.0.0.1:6379`）
    - `RedisPassword`（預設空字串）
    - `SessionTTL`：從 `SESSION_TTL_SECONDS` 讀取，預設 3600 秒。
    - `MaxSessionsPerUser`：從 `MAX_SESSIONS_PER_USER` 讀取，預設 2。

- **Redis 連線與 key（`internal/infra/redis.go`）**
  - `NewRedisClient(cfg *config.Config) *redis.Client`：使用 `github.com/redis/go-redis/v9`。
  - Key 設計：
    - `sess:{sessionID}`（Hash）：
      - 欄位：`user_id`, `created_at`, `expires_at`, `ip`, `user_agent`
      - 同時會設定 `ExpireAt(expires_at)`。
    - `user_sess:{userID}`（Sorted Set）：
      - member：`sessionID`
      - score：`created_at` 的 UNIX time。

- **Sessions 表與 sqlc**
  - `db/migrations/002_add_sessions.sql`：
    - `sessions (id TEXT PRIMARY KEY, user_id, created_at, expires_at, revoked_at, revoked_by)`。
  - `db/queries/sessions.sql`：
    - `CreateSession`：登入時記錄一筆 session。
    - `RevokeSession`：logout 或被踢時標記 `revoked_at` / `revoked_by`。

- **SessionService（`internal/session/service.go`）**
  - `Login(ctx, username, password, meta) (user db.User, sessionID string, expiresAt time.Time, err error)`：
    - 驗證帳密（延用 Phase 1 的 bcrypt）。
    - 使用 `ZCard(user_sess:{userID})` 判斷目前 session 數：
      - 若 `>= MaxSessionsPerUser`：
        - `ZRange(..., 0, 0)` 取最舊 sessionID。
        - 刪除 `sess:{oldSID}` 與 `user_sess:{userID}` 裡的成員。
        - 呼叫 `RevokeSession(id=oldSID, revoked_by='system:limit')`。
    - 產生新的 `sessionID = uuid.NewString()`，`expiresAt = now + SessionTTL`。
    - 寫入 Redis：
      - `HSet sess:{sid}` + `ExpireAt(sess:{sid}, expiresAt)`。
      - `ZAdd user_sess:{uid}`（score = `created_at unix`）。
    - 寫入 SQLite `sessions` 表作為 audit。
  - `Logout(ctx, userID, sessionID)`：
    - 刪除 `sess:{sid}` 與 `user_sess:{uid}` 成員。
    - 呼叫 `RevokeSession(id=sid, revoked_by='user')`（若該 sid 不存在則忽略）。
  - `IsSessionValid(ctx, userID, sessionID)`：
    - `HGetAll(sess:{sid})`，若不存在 → `false`。
    - 檢查 `user_id` 是否等於呼叫者的 userID，不符也視為無效。

- **JWT Claims 與發 token（`internal/token/jwt.go`）**
  - Claims 新增欄位：
    - `SessionID string 'json:"sid"'`。
  - 新增方法：
    - `GenerateWithSession(userID int64, sessionID string, expiresAt time.Time)`：
      - `sub = userID`、`sid = sessionID`、`exp = expiresAt`。

- **JWT Middleware 雙層驗證（`internal/middleware/auth_jwt.go`）**
  - 建構子改為：
    - `NewAuthJWTMiddleware(jwtMgr *token.Manager, sessSvc *session.SessionService)`.
  - 流程：
    1. 從 `Authorization: Bearer <token>` 取出 JWT。
    2. `jwtMgr.Parse` 驗證簽章與 `exp`，取得 `userID` 與 `sessionID`。
    3. 若 `sessionID` 為空 → 401。
    4. 呼叫 `sessSvc.IsSessionValid(userID, sessionID)`：
       - 若 Redis 無這個 session 或 user_id 不符 → 401。
    5. 將 `userID`、`sessionID` 設到 Gin context。

- **Auth Handlers 更新（`internal/http/handler_auth.go`）**
  - `AuthHandler` 新增依賴：
    - `sessSvc *session.SessionService`
    - `tokenTTL time.Duration`（通常與 `SessionTTL` 相同）
  - `POST /auth/login`：
    - 呼叫 `sessSvc.Login(...)` 取得 `user, sessionID, expiresAt`。
    - 呼叫 `jwtMgr.GenerateWithSession(user.ID, sessionID, expiresAt)`。
    - 回傳：
      ```json
      {
        "access_token": "<JWT>",
        "expires_in": 3600
      }
      ```
  - `POST /auth/logout`（新路由，需要 JWT）：
    - 從 context 取得 `userID`、`sessionID`（middleware 已填好）。
    - 呼叫 `sessSvc.Logout`。
    - 回傳 `{ "ok": true }`。
  - `GET /me`：
    - 維持原邏輯：從 context 拿 `userID`，用 `GetUserByID` 查 DB，回使用者資訊。
    - 但現在已經確保該 session 同時通過 JWT + Redis 驗證。

- **Router（`internal/http/router.go`）**
  - `NewRouter(q, jwtMgr, sessSvc, tokenTTL)`：
    - 公開路由：
      - `POST /auth/signup`
      - `POST /auth/login`
    - 需 JWT + Redis session 的路由：
      - 使用 `middleware.NewAuthJWTMiddleware(jwtMgr, sessSvc)`。
      - `GET /me`
      - `POST /auth/logout`

- **main（`cmd/api/main.go`）**
  - 初始化流程：
    - `cfg := config.Load()`
    - 開啟 SQLite、`runMigrations` 執行 `db/migrations/*.sql`（包含 `001_init.sql` 與 `002_add_sessions.sql`）。
    - `q := db.New(sqlDB)`
    - `rdb := infra.NewRedisClient(cfg)`
    - `sessSvc := session.NewSessionService(q, rdb, cfg)`
    - `jwtMgr := token.NewManager(cfg.JWTSecret, cfg.SessionTTL)`
    - `router := httpapi.NewRouter(q, jwtMgr, sessSvc, cfg.SessionTTL)`

---

### 如何在本機啟動 Phase 2

1. 啟動 Redis（若本機尚未有 Redis）：

```bash
docker run --rm -p 6379:6379 redis:7.4-alpine
```

2. 啟動 API（Phase 2 版）：

```bash
cd /Users/user/session-service

go mod tidy
sqlc generate

APP_HTTP_ADDR=":8080" \
APP_DB_PATH="./data/app.db" \
APP_JWT_SECRET="dev-secret-change-me" \
REDIS_ADDR="127.0.0.1:6379" \
SESSION_TTL_SECONDS=3600 \
MAX_SESSIONS_PER_USER=2 \
go run ./cmd/api
```

---

### Phase 2 測試腳本範例

#### 1. Signup + Login + /me

```bash
BASE_URL="http://localhost:8080"

# 建立使用者
curl -s -X POST "$BASE_URL/auth/signup" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"password123"}'

# 登入取得 access_token
LOGIN_RES=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"password123"}')

echo "$LOGIN_RES"

TOKEN=$(echo "$LOGIN_RES" | jq -r '.access_token')
echo "TOKEN=$TOKEN"

# 使用 token 呼叫 /me
curl -s "$BASE_URL/me" \
  -H "Authorization: Bearer $TOKEN"
```

登入成功後，可以在 Redis 檢查：

```bash
redis-cli
> KEYS sess:*
> KEYS user_sess:*
> HGETALL sess:<session-id>
> ZRANGE user_sess:<user-id> 0 -1 WITHSCORES
```

#### 2. Logout 後 /me 失效

```bash
# 使用上一步的 TOKEN

# 呼叫 /auth/logout
curl -s -X POST "$BASE_URL/auth/logout" \
  -H "Authorization: Bearer $TOKEN"

# 再呼叫 /me（預期 401）
curl -i "$BASE_URL/me" \
  -H "Authorization: Bearer $TOKEN"
```

在 Redis 應該看不到該 `sess:{sid}`，`user_sess:{uid}` 內也不再有這個 sessionID。

#### 3. 同時登入數限制測試（MaxSessionsPerUser = 2）

```bash
BASE_URL="http://localhost:8080"

TOKENS=()
for i in 1 2 3; do
  RES=$(curl -s -X POST "$BASE_URL/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"alice","password":"password123"}')
  TOKENS+=("$(echo "$RES" | jq -r '.access_token')")
  echo "Login $i: $RES"
done

TOKEN1=${TOKENS[0]}
TOKEN2=${TOKENS[1]}
TOKEN3=${TOKENS[2]}

echo "T1=$TOKEN1"
echo "T2=$TOKEN2"
echo "T3=$TOKEN3"

# 在 Redis 檢查，只應存在 2 個 session（最新的兩個）
redis-cli ZRANGE user_sess:<user-id> 0 -1
```

- 理想情況：
  - 第 3 次登入時，最舊的 session（第一個登入）會被刪除。
  - `user_sess:{userID}` 只保留 2 個 sessionID。

驗證 token 是否有效：

```bash
# 最舊的 token（預期 401）
curl -i "$BASE_URL/me" \
  -H "Authorization: Bearer $TOKEN1"

# 另外兩個 token（預期 200）
curl -i "$BASE_URL/me" \
  -H "Authorization: Bearer $TOKEN2"

curl -i "$BASE_URL/me" \
  -H "Authorization: Bearer $TOKEN3"
```

若最舊的 token 收到 401，而後兩個仍可正常呼叫 `/me`，代表 Redis Session 管理與同時登入數限制已正常運作。 

---

## Phase 3 - Asynq 與管理端 API

Phase 3 在前兩階段的基礎上，加入：

- 基於 Redis 的 **Asynq 延遲任務**（自動 session 過期、登入稽核 log）
- **login_events** 稽核表
- **封鎖 / 解封 user** 與 **後台管理 API**

---

### Phase 3 新增/變更重點

- **Config（`internal/config/config.go`）**
  - 新增：
    - `AsynqConcurrency int`：Asynq worker 併發數，預設 `10`，來自 `ASYNQ_CONCURRENCY`。
    - `AdminAPIKey string`：管理端 API 的簡易密鑰，來自 `ADMIN_API_KEY`，預設 `"dev-admin"`。

- **Asynq Client（`internal/infra/asynq_client.go`）**
  - 任務類型常數：
    - `TaskTypeSessionExpire = "session:expire"`
    - `TaskTypeLoginAudit  = "login:audit"`
  - Payload：
    - `SessionExpirePayload{ SessionID string, UserID int64 }`
    - `LoginAuditPayload{ UserID *int64, Username string, Success bool, Reason string, IP string, UserAgent string }`
  - `NewAsynqClient(cfg *config.Config) *asynq.Client`：重用 Redis addr/password。
  - Helper：
    - `EnqueueSessionExpire(ctx, client, sessionID, userID, processAt)`：在 `processAt` 時自動執行 `session:expire`。
    - `EnqueueLoginAudit(ctx, client, payload)`：即時送出 `login:audit` 任務。

- **Asynq Worker（`cmd/worker/main.go`）**
  - 共用同一份 `config` 與 SQLite / Redis 設定。
  - 啟動 Asynq Server：
    - 使用 `cfg.RedisAddr` / `cfg.RedisPassword`。
    - `Concurrency = cfg.AsynqConcurrency`。
  - Handler：
    - `session:expire`：
      - 讀 payload `{session_id, user_id}`。
      - 檢查 `sess:{sid}` 是否仍存在：
        - 若不存在 → 視為已處理過（可能手動 logout 或被踢），直接 return nil。
        - 若存在：
          - 刪除 `sess:{sid}` hash。
          - 從 `user_sess:{uid}` ZSet 中移除該 sid。
          - 呼叫 `RevokeSession(id=sid, revoked_by="system:expire")` 更新 SQLite。
    - `login:audit`：
      - 讀 payload `{ user_id?, username, success, reason, ip, user_agent }`。
      - 寫入 `login_events` 表，作為登入稽核紀錄（目前以 raw SQL `INSERT` 實作）。
  - 具備優雅關閉：收到 SIGINT/SIGTERM 時呼叫 `srv.Shutdown()`。

- **DB & sqlc**
  - `db/migrations/003_add_login_events.sql`：
    - `login_events` 表：
      - `id`, `user_id NULL`, `username`, `success`, `reason`, `ip`, `user_agent`, `created_at`。
  - `db/queries/login_events.sql` + `internal/db/login_events.sql.go`：
    - `InsertLoginEvent` 可供之後切換到 sqlc 使用（目前 worker 先用 raw SQL）。
  - `db/migrations/004_add_user_ban.sql`：
    - 在 `users` 表新增 `is_banned BOOLEAN NOT NULL DEFAULT 0`。
  - `db/queries/users.sql`：
    - 所有 user 查詢/建立都加入 `is_banned` 欄位。
    - 新增：
      - `BanUser :exec` → `UPDATE users SET is_banned = 1 WHERE id = ?`
      - `UnbanUser :exec` → `UPDATE users SET is_banned = 0 WHERE id = ?`

- **Redis banned flag（`internal/infra/redis.go`）**
  - 新增：
    - `BannedUserKey(userID int64) string` → `banned_user:{userID}`；存在即代表被 ban。

- **SessionService 擴充（`internal/session/service.go`）**
  - 結構：
    - `type SessionService struct { q *db.Queries; rdb *redis.Client; cfg *config.Config; asynqClient *asynq.Client }`
    - 由 `NewSessionService(q, rdb, cfg, asynqClient)` 建立。
  - 登入流程 `Login(ctx, username, password, meta)` 新增：
    - 登入前：
      - 使用 `GetUserByUsername` 查 user。
      - 若 user 不存在 → 回 `ErrInvalidCredentials`，並送出 `login:audit`（`user_id=nil, reason="user_not_found"`）。
      - 若 `u.IsBanned`（DB）或 Redis `banned_user:{uid}` 存在 → 回 `ErrUserBanned`，送出 `login:audit`（`reason="banned_db"` 或 `reason="banned_redis"`）。
      - 密碼錯誤 → 回 `ErrInvalidCredentials`，送出 `login:audit`（`reason="wrong_password"`）。
    - 登入成功後（原 Phase 2 Redis + SQLite 邏輯不變）：
      - 產生 sid，寫 Redis `sess:{sid}` + `user_sess:{uid}`，並 `CreateSession`。
      - 排 Asynq 任務：
        - `session:expire` 在 `expiresAt` 執行。
        - `login:audit`：`success=true, reason="ok"`。
  - Admin / GM 功能：
    - `ListActiveSessions(ctx, userID)`：
      - 從 `user_sess:{uid}` 讀 sid 列表，依序讀 `sess:{sid}` hash，輸出陣列：
        - `[{ "session_id": "...", "ip": "...", "user_agent": "..." }, ...]`。
    - `KickSession(ctx, userID, sessionID)`：
      - 刪除 `sess:{sid}` + `ZREM user_sess:{uid} sid`。
      - 呼叫 `RevokeSession(id=sid, revoked_by="admin:kick")`。
    - `KickAllSessions(ctx, userID)`：
      - 取得該 user 所有 sid，逐一呼叫 `KickSession`。
    - `BanUser(ctx, userID)`：
      - 呼叫 `BanUser`（DB），設 `is_banned = 1`。
      - `SET banned_user:{uid} "1"`（永久 ban，無 TTL）。
      - `KickAllSessions` 踢掉所有現有 session。
    - `UnbanUser(ctx, userID)`：
      - 呼叫 `UnbanUser`（DB），`is_banned = 0`。
      - `DEL banned_user:{uid}`。

- **Admin API Key Middleware（`internal/middleware/admin_api_key.go`）**
  - `NewAdminAPIKeyMiddleware(adminKey string)`：
    - 從 Header `X-Admin-Token` 讀取值：
      - 若 `adminKey` 非空，且 header 不相符 → 回傳 403 `{ "error": "forbidden" }`。
      - 若 `adminKey` 為空，則直接放行（僅建議在本機開發模式）。

- **Admin 路由與 Handler（`internal/http/router.go`, `internal/http/handler_admin.go`）**
  - `NewRouter(q, jwtMgr, sessSvc, tokenTTL, adminAPIKey)`：
    - `/auth` & `/me`：
      - 同 Phase 2，但底層登入邏輯改走 Asynq-enhanced SessionService。
    - `/admin` group：
      - 掛上 `NewAdminAPIKeyMiddleware(adminAPIKey)`。
      - 路由：
        - `GET  /admin/users/:id/sessions` → `ListUserSessions`：
          - 回傳 Redis 中該 user 所有 active sessions（`session_id`, `ip`, `user_agent`）。
        - `POST /admin/users/:id/kick` → `KickUserSessions`：
          - Body 可為：
            - `{ "session_id": "..." }` → 踢掉單一 session。
            - `{ "all": true }` → 踢掉所有 session。
        - `POST /admin/users/:id/ban` → `BanUser`。
        - `POST /admin/users/:id/unban` → `UnbanUser`。

- **API / Worker 啟動（`cmd/api/main.go`, `cmd/worker/main.go`）**
  - `cmd/api/main.go`：
    - 除原本 SQLite / Redis 初始化外，新增：
      - `asynqClient := infra.NewAsynqClient(cfg)`，傳入 `NewSessionService`。
      - `NewRouter(..., cfg.AdminAPIKey)` 啟用 admin API。
  - `cmd/worker/main.go`：
    - 使用同一份 `config` 與 DBPath / RedisAddr。
    - 啟動 Asynq Server 及上述兩個 handler。

---

### 啟動 Phase 3（API + Worker）

1. 啟動 Redis：

```bash
docker run --rm -p 6379:6379 redis:7.4-alpine
```

2. 啟動 API：

```bash
cd /Users/user/session-service

go mod tidy
sqlc generate

APP_HTTP_ADDR=":8080" \
APP_DB_PATH="./data/app.db" \
APP_JWT_SECRET="dev-secret-change-me" \
REDIS_ADDR="127.0.0.1:6379" \
SESSION_TTL_SECONDS=3600 \
MAX_SESSIONS_PER_USER=2 \
ADMIN_API_KEY="dev-admin" \
go run ./cmd/api
```

3. 啟動 Worker（另一個 terminal）：

```bash
cd /Users/user/session-service

APP_DB_PATH="./data/app.db" \
REDIS_ADDR="127.0.0.1:6379" \
ASYNQ_CONCURRENCY=10 \
go run ./cmd/worker
```

---

### Admin API 測試腳本範例

假設：

- User ID 為 `1`
- `ADMIN_API_KEY="dev-admin"`

#### 1. 列出某 user 的活躍 sessions

```bash
BASE_URL="http://localhost:8080"
ADMIN_TOKEN="dev-admin"

curl -s "$BASE_URL/admin/users/1/sessions" \
  -H "X-Admin-Token: $ADMIN_TOKEN"
```

回應範例：

```json
{
  "sessions": [
    {
      "session_id": "1e2b3c4d-...",
      "ip": "127.0.0.1",
      "user_agent": "curl/8.0.1"
    }
  ]
}
```

#### 2. 踢掉單一 session

```bash
curl -s -X POST "$BASE_URL/admin/users/1/kick" \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: $ADMIN_TOKEN" \
  -d '{"session_id":"1e2b3c4d-..."}'
```

#### 3. 踢掉該 user 所有 sessions

```bash
curl -s -X POST "$BASE_URL/admin/users/1/kick" \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: $ADMIN_TOKEN" \
  -d '{"all":true}'
```

#### 4. 封鎖 / 解封 user

```bash
# 封鎖 user 1（會更新 DB.is_banned、設 banned_user:{1}，並踢掉所有 sessions）
curl -s -X POST "$BASE_URL/admin/users/1/ban" \
  -H "X-Admin-Token: $ADMIN_TOKEN"

# 解封 user 1
curl -s -X POST "$BASE_URL/admin/users/1/unban" \
  -H "X-Admin-Token: $ADMIN_TOKEN"
```

被封鎖的 user 日後嘗試登入時：

- SessionService 會檢查 DB `is_banned` 與 Redis `banned_user:{uid}`：
  - 失敗並回傳錯誤（如 `user is banned`）。
  - 同時透過 `login:audit` 任務寫入 `login_events`，reason 會是 `banned_db` 或 `banned_redis`。


