package security

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/wyfcoding/pkg/contextx"
)

// APIKeyValidator 定义了验证 API Key 的接口
type APIKeyValidator interface {
	ValidateKey(ctx context.Context, key, secret string) (userID string, scopes string, err error)
}

// AuthInterceptor 提供了 gRPC 认证拦截器
type AuthInterceptor struct {
	validator APIKeyValidator
}

func NewAuthInterceptor(v APIKeyValidator) *AuthInterceptor {
	return &AuthInterceptor{validator: v}
}

// Unary 是一元请求的认证拦截器
func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// 1. 提取 Metadata 中的 API Key
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.Unauthenticated, "metadata is missing")
		}

		keys := md.Get("x-api-key")
		secrets := md.Get("x-api-secret")
		if len(keys) == 0 || len(secrets) == 0 {
			return nil, status.Errorf(codes.Unauthenticated, "api key or secret is missing")
		}

		// 2. 校验 Key
		userID, scopes, err := i.validator.ValidateKey(ctx, keys[0], secrets[0])
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid api key: %v", err)
		}

		// 3. 注入用户信息到 Context
		newCtx := contextx.WithUserID(ctx, userID)
		newCtx = contextx.WithScopes(newCtx, scopes)

		return handler(newCtx, req)
	}
}

// ScopeRequired 检查 Context 中是否包含指定的 Scope
func ScopeRequired(ctx context.Context, requiredScope string) error {
	scopes := contextx.GetScopes(ctx)
	if scopes == "" {
		return status.Errorf(codes.PermissionDenied, "no scopes found in context")
	}

	if !strings.Contains(scopes, requiredScope) {
		return status.Errorf(codes.PermissionDenied, "scope %s is required", requiredScope)
	}

	return nil
}
