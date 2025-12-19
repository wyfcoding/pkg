package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5" // 导入JWT库，版本v5。
)

// AdminClaims 定义了管理员JWT的自定义载荷结构。
type AdminClaims struct {
	AdminID  uint64 `json:"admin_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

// GenerateAdminToken 生成一个用于管理员用户的JWT (JSON Web Token)。
// 这个token包含了管理员的基本信息，并设置了签发者和过期时间，用于认证和授权。
// adminID: 管理员用户的唯一ID。
// username: 管理员的用户名。
// email: 管理员的邮箱地址。
// secret: 用于签名JWT的密钥。
// issuer: JWT的签发者（例如，"your-service-name"）。
// expireSeconds: JWT的过期时间，以秒为单位。
// 返回值：生成的JWT字符串和可能发生的错误。
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

	// 使用HS256签名方法和自定义Claims创建一个新的Token。
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// 使用秘密密钥对Token进行签名，并返回签名的JWT字符串。
	return token.SignedString([]byte(secret))
}

// ParseAdminToken 解析JWT字符串，并返回自定义的AdminClaims。
// tokenString: 待解析的JWT字符串。
// secretKey: 用于验证JWT签名的密钥。
// 返回值：解析出的AdminClaims指针和可能发生的错误。
func ParseAdminToken(tokenString string, secretKey string) (*AdminClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AdminClaims{}, func(token *jwt.Token) (interface{}, error) {
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

	if claims, ok := token.Claims.(*AdminClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrTokenInvalid
}
