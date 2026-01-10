// Package jwt 提供基于 HS256 算法的 JSON Web Token 签发与解析功能，支持 RBAC 角色注入。
package jwt

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenMalformed   = errors.New("token format is invalid")    // 令牌格式错误。
	ErrTokenExpired     = errors.New("token has expired")          // 令牌已过期。
	ErrTokenNotValidYet = errors.New("token is not active yet")    // 令牌尚未生效。
	ErrTokenInvalid     = errors.New("token signature is invalid") // 令牌签名无效或校验不通过。
)

// MyCustomClaims 定义了 JWT 的 Payload 部分，包含用户信息与权限角色。
type MyCustomClaims struct { //nolint:govet // 已按照 JWT 标准 Payload 结构对齐。
	UserID   uint64   `json:"user_id"`  // 用户唯一标识。
	Username string   `json:"username"` // 用户登录名。
	Roles    []string `json:"roles"`    // 用户所属的角色列表（用于 RBAC）。
	jwt.RegisteredClaims
}

// GenerateToken 是系统中标准且唯一的 JWT 生成入口。
// 强制要求注入 roles 列表以支持下游服务的权限校验。
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

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

// ParseToken 提供安全的 JWT 解析能力。
// 流程：解析结构 -> 验证签名算法 -> 验证有效期 -> 返回 Claims。
func ParseToken(tokenString, secretKey string) (*MyCustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &MyCustomClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, token.Header["alg"])
		}
		return []byte(secretKey), nil
	})
	if err != nil {
		slog.Warn("jwt parse failed", "error", err)
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		}
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		if errors.Is(err, jwt.ErrTokenNotValidYet) {
			return nil, ErrTokenNotValidYet
		}
		return nil, ErrTokenInvalid
	}

	if claims, ok := token.Claims.(*MyCustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrTokenInvalid
}
