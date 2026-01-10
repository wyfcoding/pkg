// Package jwt 封装了基于 Go-JWT 的令牌生成、解析与校验逻辑，支持自定义 Claims 与多种签名算法。
package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrInvalidToken 令牌无效或签名不匹配。
	ErrInvalidToken = errors.New("invalid token")
	// ErrExpiredToken 令牌已过期。
	ErrExpiredToken = errors.New("token expired")
	// ErrTokenMalformed 令牌格式错误。
	ErrTokenMalformed = errors.New("token is malformed")
)

// MyCustomClaims 定义了 JWT 的 Payload 部分，包含用户信息与权限角色。
type MyCustomClaims struct {
	jwt.RegisteredClaims
	Roles    []string `json:"roles"`
	Username string   `json:"username"`
	UserID   uint64   `json:"user_id"`
}

// GenerateToken 是系统中标准且唯一的 JWT 生成入口。
func GenerateToken(userID uint64, username string, roles []string, secret string, issuer string, expireDuration time.Duration) (string, error) {
	claims := MyCustomClaims{
		UserID:   userID,
		Username: username,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expireDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

// ParseToken 解析并验证 JWT 字符串，返回自定义 Claims。
func ParseToken(tokenString string, secret string) (*MyCustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &MyCustomClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}

		return nil, ErrInvalidToken
	}

	if claims, ok := token.Claims.(*MyCustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}
