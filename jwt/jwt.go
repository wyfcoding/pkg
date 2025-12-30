package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenMalformed   = errors.New("token is malformed")
	ErrTokenExpired     = errors.New("token is expired")
	ErrTokenNotValidYet = errors.New("token not valid yet")
	ErrTokenInvalid     = errors.New("token is invalid")
)

// MyCustomClaims 定义了JWT的载荷结构，包含已优化的 Roles 字段
type MyCustomClaims struct {
	UserID               uint64   `json:"user_id"`
	Username             string   `json:"username"`
	Roles                []string `json:"roles"` // 【保留优化】：支持 RBAC
	jwt.RegisteredClaims
}

// GenerateToken 原始签名函数 (保持不变，以兼容现有调用)
func GenerateToken(userID uint64, username, secretKey, issuer string, expires time.Duration, method jwt.SigningMethod) (string, error) {
	// 内部调用增强版函数，roles 传空
	return GenerateTokenWithRoles(userID, username, nil, secretKey, issuer, expires, method)
}

// GenerateTokenWithRoles 【新增】：增强版函数，支持传入 Roles
func GenerateTokenWithRoles(userID uint64, username string, roles []string, secretKey, issuer string, expires time.Duration, method jwt.SigningMethod) (string, error) {
	if method == nil {
		method = jwt.SigningMethodHS256
	}

	expireTime := time.Now().Add(expires)
	claims := MyCustomClaims{
		UserID:   userID,
		Username: username,
		Roles:    roles, // 注入角色
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    issuer,
		},
	}

	token := jwt.NewWithClaims(method, claims)
	return token.SignedString([]byte(secretKey))
}

// ParseToken 解析JWT字符串
func ParseToken(tokenString string, secretKey string) (*MyCustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &MyCustomClaims{}, func(token *jwt.Token) (any, error) {
		return []byte(secretKey), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		} else if errors.Is(err, jwt.ErrTokenExpired) || errors.Is(err, jwt.ErrTokenNotValidYet) {
			return nil, ErrTokenExpired 
		} else {
			return nil, ErrTokenInvalid
		}
	}

	if claims, ok := token.Claims.(*MyCustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrTokenInvalid
}