package jwt

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenMalformed   = errors.New("token format is invalid")
	ErrTokenExpired     = errors.New("token has expired")
	ErrTokenNotValidYet = errors.New("token is not active yet")
	ErrTokenInvalid     = errors.New("token signature is invalid")
)

// MyCustomClaims 定义了包含 RBAC 权限的 JWT 载荷
type MyCustomClaims struct {
	UserID   uint64   `json:"user_id"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

// GenerateToken 是项目中唯一合法的 JWT 生成函数
// 强制要求传入 roles 列表，即使为空也需传入 nil
func GenerateToken(userID uint64, username string, roles []string, secretKey, issuer string, expires time.Duration) (string, error) {
	now := time.Now()
	claims := MyCustomClaims{
		UserID:   userID,
		Username: username,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(expires)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    issuer,
		},
	}

	// 统一强制使用 HS256 签名，保证安全与一致性
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

// ParseToken 安全解析 JWT
func ParseToken(tokenString string, secretKey string) (*MyCustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &MyCustomClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secretKey), nil
	})
	if err != nil {
		slog.Warn("JWT parse failed", "error", err)
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		} else if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		} else if errors.Is(err, jwt.ErrTokenNotValidYet) {
			return nil, ErrTokenNotValidYet
		} else {
			return nil, ErrTokenInvalid
		}
	}

	if claims, ok := token.Claims.(*MyCustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrTokenInvalid
}
