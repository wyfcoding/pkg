package jwt

import (
	"errors"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AdminClaims 封装了管理后台专用令牌的 Payload 结构。
type AdminClaims struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
	AdminID uint64 `json:"admin_id"`
}

// GenerateAdminToken 构造并签发一个管理员级别的 JWT 令牌。
func GenerateAdminToken(adminID uint64, username, email, secret, issuer string, expireSeconds int64) (string, error) {
	expireTime := time.Now().Add(time.Duration(expireSeconds) * time.Second)
	claims := AdminClaims{
		AdminID:  adminID,
		Username: username,
		Email:    email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseAdminToken 解析并验证管理员 JWT 字符串。
func ParseAdminToken(tokenString, secretKey string) (*AdminClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AdminClaims{}, func(token *jwt.Token) (any, error) {
		// 验证签名方法是否符合预期（防止算法回退攻击）。
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secretKey), nil
	})
	if err != nil {
		slog.Warn("admin jwt parse failed", "error", err)
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		}
		if errors.Is(err, jwt.ErrTokenExpired) || errors.Is(err, jwt.ErrTokenNotValidYet) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(*AdminClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}
