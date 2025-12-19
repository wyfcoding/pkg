package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5" // 导入JWT库，版本v5。
)

// 定义标准的JWT错误类型，方便错误处理和区分。
var (
	ErrTokenMalformed   = errors.New("token is malformed")  // token格式不正确。
	ErrTokenExpired     = errors.New("token is expired")    // token已过期。
	ErrTokenNotValidYet = errors.New("token not valid yet") // token尚未生效。
	ErrTokenInvalid     = errors.New("token is invalid")    // token无效（例如签名不匹配）。
)

// MyCustomClaims 定义了JWT的自定义载荷 (Payload) 结构。
// 它嵌入了 `jwt.RegisteredClaims` 以包含标准的JWT字段，并添加了业务相关的字段。
type MyCustomClaims struct {
	UserID               uint64 `json:"user_id"`  // 用户ID。
	Username             string `json:"username"` // 用户名。
	jwt.RegisteredClaims        // 嵌入标准的JWT注册声明，如过期时间、签发者等。
}

// GenerateToken 生成一个JWT (JSON Web Token)。
// userID: 用户的唯一标识符。
// username: 用户名。
// secretKey: 用于签名JWT的密钥。
// issuer: JWT的签发者。
// expires: JWT的有效期。
// method: JWT的签名算法，如果为nil则默认使用HS256。
// 返回值：生成的JWT字符串和可能发生的错误。
func GenerateToken(userID uint64, username, secretKey, issuer string, expires time.Duration, method jwt.SigningMethod) (string, error) {
	// 如果没有指定签名方法，则使用默认的HS256。
	if method == nil {
		method = jwt.SigningMethodHS256
	}

	// 计算JWT的过期时间。
	expireTime := time.Now().Add(expires)
	// 创建自定义Claims。
	claims := MyCustomClaims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime), // JWT的过期时间。
			IssuedAt:  jwt.NewNumericDate(time.Now()), // JWT的签发时间。
			NotBefore: jwt.NewNumericDate(time.Now()), // JWT生效时间（通常与签发时间相同）。
			Issuer:    issuer,                         // JWT的签发者。
			// ID:        "", // JWT ID，可选。
			// Audience:  nil, // 受众，可选。
			// Subject:   "", // 主题，可选。
		},
	}

	// 使用提供的签名算法和自定义Claims创建一个新的Token对象。
	token := jwt.NewWithClaims(method, claims)

	// 使用提供的密钥对Token进行签名，并获取完整的编码后的字符串Token。
	return token.SignedString([]byte(secretKey))
}

// ParseToken 解析JWT字符串，并返回自定义的Claims。
// tokenString: 待解析的JWT字符串。
// secretKey: 用于验证JWT签名的密钥。
// 返回值：解析出的MyCustomClaims指针和可能发生的错误。
func ParseToken(tokenString string, secretKey string) (*MyCustomClaims, error) {
	// 使用jwt.ParseWithClaims解析token字符串。
	// 第一个参数是token字符串，第二个参数是用于解析Claims的结构体实例，
	// 第三个参数是一个回调函数，用于提供验证签名的密钥。
	token, err := jwt.ParseWithClaims(tokenString, &MyCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 返回用于验证签名的密钥。
		return []byte(secretKey), nil
	})

	// 处理解析过程中可能发生的各种错误。
	if err != nil {
		// jwt.ErrTokenMalformed: token格式不正确。
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		} else if errors.Is(err, jwt.ErrTokenExpired) || errors.Is(err, jwt.ErrTokenNotValidYet) {
			// jwt.ErrTokenExpired: token已过期。
			// jwt.ErrTokenNotValidYet: token尚未生效。
			return nil, ErrTokenExpired // 将两种过期或未生效错误统一为ErrTokenExpired。
		} else {
			// 其他类型的错误，例如签名无效。
			return nil, ErrTokenInvalid
		}
	}

	// 校验token的有效性，并提取自定义Claims。
	// token.Valid 会自动检查过期时间、生效时间等标准Claim。
	if claims, ok := token.Claims.(*MyCustomClaims); ok && token.Valid {
		return claims, nil // 解析成功且Token有效，返回Claims。
	}

	return nil, ErrTokenInvalid // Token无效。
}
