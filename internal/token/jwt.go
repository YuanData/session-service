package token

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 定義我們在 JWT 中使用的 claims。
// - sub: user ID
// - exp: 過期時間
// - iat: 發行時間
type Claims struct {
	UserID int64 `json:"sub"`
	jwt.RegisteredClaims
}

// Manager 負責產生與解析 JWT。
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager 建立一個新的 JWT Manager。
// ttl 代表 access token 的存活時間（例如 24h）。
func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

// Generate 為指定 user 產生一顆 JWT。
func (m *Manager) Generate(userID int64) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Parsed 包裝解析後的結果，方便之後擴充。
type Parsed struct {
	Token  *jwt.Token
	Claims *Claims
}

var (
	// ErrInvalidToken 代表 token 無效或簽章錯誤。
	ErrInvalidToken = errors.New("invalid token")
)

// Parse 解析並驗證 JWT。
func (m *Manager) Parse(tokenStr string) (*Parsed, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))

	tok, err := parser.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, ErrInvalidToken
	}

	return &Parsed{
		Token:  tok,
		Claims: claims,
	}, nil
}


