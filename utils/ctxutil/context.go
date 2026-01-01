package ctxutil

import (
	"context"
)

type contextKey string

const (
	UserIDKey   contextKey = "user_id"
	TenantIDKey contextKey = "tenant_id"
	RoleKey     contextKey = "user_role"
	IPKey       contextKey = "client_ip"
	UAKey       contextKey = "user_agent"
)

// WithUserID 注入用户 ID
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetUserID 提取用户 ID
func GetUserID(ctx context.Context) string {
	if val, ok := ctx.Value(UserIDKey).(string); ok {
		return val
	}
	return ""
}

// WithIP 注入客户端 IP
func WithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, IPKey, ip)
}

// GetIP 提取客户端 IP
func GetIP(ctx context.Context) string {
	if val, ok := ctx.Value(IPKey).(string); ok {
		return val
	}
	return "0.0.0.0"
}

// WithUserAgent 注入 User-Agent
func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, UAKey, ua)
}

// GetUserAgent 提取 User-Agent
func GetUserAgent(ctx context.Context) string {
	if val, ok := ctx.Value(UAKey).(string); ok {
		return val
	}
	return "Unknown"
}

// WithRole 注入用户角色
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, RoleKey, role)
}

// GetRole 提取用户角色
func GetRole(ctx context.Context) string {
	if val, ok := ctx.Value(RoleKey).(string); ok {
		return val
	}
	return ""
}

// WithTenantID 注入租户 ID (多租户架构必备)
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// GetTenantID 提取租户 ID
func GetTenantID(ctx context.Context) string {
	if val, ok := ctx.Value(TenantIDKey).(string); ok {
		return val
	}
	return ""
}
