// Package contextx 提供了一组用于安全地在 context.Context 中注入与提取业务上下文信息（如用户 ID、租户、IP、UA 等）的工具函数。
// 它通过使用私有类型作为 Key，有效防止了跨包的 Key 冲突。
package contextx

import (
	"context"
)

type contextKey string

const (
	UserIDKey   contextKey = "user_id"    // 用户唯一标识 Key
	TenantIDKey contextKey = "tenant_id"  // 租户 ID Key
	RoleKey     contextKey = "user_role"  // 用户角色 Key
	IPKey       contextKey = "client_ip"  // 客户端 IP Key
	UAKey       contextKey = "user_agent" // 用户代理 Key
	DBTxKey     contextKey = "db_tx"      // 数据库事务 Key
)

// WithTx 将 GORM 事务实例注入到 Context 中。
func WithTx(ctx context.Context, tx any) context.Context {
	return context.WithValue(ctx, DBTxKey, tx)
}

// GetTx 从 Context 中尝试提取 GORM 事务实例。
func GetTx(ctx context.Context) any {
	return ctx.Value(DBTxKey)
}

// WithUserID 将用户 ID 注入到给定的 Context 中。
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetUserID 从 Context 中尝试提取用户 ID，若不存在则返回空字符串。
func GetUserID(ctx context.Context) string {
	if val, ok := ctx.Value(UserIDKey).(string); ok {
		return val
	}
	return ""
}

// WithIP 将客户端 IP 地址注入到 Context 中。
func WithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, IPKey, ip)
}

// GetIP 从 Context 中尝试提取客户端 IP，若不存在则返回默认回环地址。
func GetIP(ctx context.Context) string {
	if val, ok := ctx.Value(IPKey).(string); ok {
		return val
	}
	return "0.0.0.0"
}

// WithUserAgent 将 User-Agent 信息注入到 Context 中。
func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, UAKey, ua)
}

// GetUserAgent 从 Context 中尝试提取 User-Agent，若不存在则返回 "Unknown"。
func GetUserAgent(ctx context.Context) string {
	if val, ok := ctx.Value(UAKey).(string); ok {
		return val
	}
	return "Unknown"
}

// WithRole 将用户角色信息注入到 Context 中。
func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, RoleKey, role)
}

// GetRole 从 Context 中尝试提取用户角色，若不存在则返回空字符串。
func GetRole(ctx context.Context) string {
	if val, ok := ctx.Value(RoleKey).(string); ok {
		return val
	}
	return ""
}

// WithTenantID 将租户 ID 注入到 Context 中，支持多租户架构。
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// GetTenantID 从 Context 中尝试提取租户 ID，若不存在则返回空字符串。
func GetTenantID(ctx context.Context) string {
	if val, ok := ctx.Value(TenantIDKey).(string); ok {
		return val
	}
	return ""
}
