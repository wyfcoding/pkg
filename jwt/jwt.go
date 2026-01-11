// Package jwt 封装了基于 Go-JWT 的令牌生成、解析与校验逻辑，支持自定义 Claims 与多种签名算法。
package jwt

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrUnexpectedSigningMethod 签名算法不匹配错误.
	ErrUnexpectedSigningMethod = errors.New("unexpected signing method")
	// ErrInvalidToken 令牌无效或签名不匹配。
	ErrInvalidToken = errors.New("invalid token")
	// ErrTokenInvalid 令牌无效（别名，保持兼容性）。
	ErrTokenInvalid = ErrInvalidToken
	// ErrExpiredToken 令牌已过期。
	ErrExpiredToken = errors.New("token expired")
	// ErrTokenExpired 令牌已过期（别名，保持兼容性）。
	ErrTokenExpired = ErrExpiredToken
	// ErrTokenMalformed 令牌格式错误。
	ErrTokenMalformed = errors.New("token malformed")

	tokenCache sync.Map // 用于缓存解析后的 *MyCustomClaims
)

// MyCustomClaims 定义了 JWT 的 Payload 部分.
type MyCustomClaims struct {
	jwt.RegisteredClaims
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	UserID   uint64   `json:"user_id"`
}

// GenerateToken 是系统中标准且唯一的 JWT 生成入口.
func GenerateToken(userID uint64, username string, roles []string, secret, issuer string, expireDuration time.Duration) (string, error) {
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

// ParseToken 解析并验证 JWT 字符串，带本地高性能缓存。
func ParseToken(tokenString, secret string) (*MyCustomClaims, error) {
	// 1. 尝试从本地缓存读取
	if val, ok := tokenCache.Load(tokenString); ok {
		claims := val.(*MyCustomClaims)
		// 仍然需要检查过期时间
		if time.Now().Before(claims.ExpiresAt.Time) {
			return claims, nil
		}
		// 已过期，从缓存中移除
		tokenCache.Delete(tokenString)
	}

	// 2. 缓存未命中或已过期，执行完整解析与验签
	token, err := jwt.ParseWithClaims(tokenString, &MyCustomClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: %v", ErrUnexpectedSigningMethod, token.Header["alg"])
		}

		return []byte(secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		}

		return nil, ErrTokenInvalid
	}

	if claims, ok := token.Claims.(*MyCustomClaims); ok && token.Valid {
		// 3. 验证成功，存入缓存
		tokenCache.Store(tokenString, claims)

		return claims, nil
	}

	return nil, ErrTokenInvalid
}
