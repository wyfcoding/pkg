// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
// 生成摘要:
// 1) 幂等中间件增加可配置选项与更安全的 Key 组合策略.
// 2) 返回缓存命中标识并补齐请求上下文日志输出.
// 假设:
// 1) 幂等 Key 由客户端传入，结合租户/用户/路径后可避免跨业务冲突。
package middleware

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/idempotency"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/response"

	"github.com/gin-gonic/gin"
)

const (
	// IdempotencyKeyHeader 幂等键请求头。
	IdempotencyKeyHeader = "X-Idempotency-Key"
	// IdempotencyCacheHeader 幂等命中标识响应头。
	IdempotencyCacheHeader = "X-Idempotency-Cache"
)

// IdempotencyOptions 定义幂等中间件的可配置项。
type IdempotencyOptions struct {
	Header   string        // 客户端幂等 Key 请求头名称。
	LockTTL  time.Duration // 处理中状态的锁定时长。
	CacheTTL time.Duration // 成功响应缓存时长。
	Methods  []string      // 仅对指定方法启用幂等控制（空表示全量）。
}

// responseBodyWriter 用于捕获 Gin 的响应内容。
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// IdempotencyMiddleware 构造一个用于 Gin 框架的通用幂等性控制中间件。
// 参数说明：
//   - manager: 具体的幂等状态管理实现（通常基于 Redis）。
//   - ttl: 业务成功处理后，响应快照在缓存中的保留时长。
func IdempotencyMiddleware(manager idempotency.Manager, ttl time.Duration) gin.HandlerFunc {
	return IdempotencyMiddlewareWithOptions(manager, IdempotencyOptions{CacheTTL: ttl})
}

// IdempotencyMiddlewareWithOptions 构造带配置项的幂等控制中间件。
func IdempotencyMiddlewareWithOptions(manager idempotency.Manager, opts IdempotencyOptions) gin.HandlerFunc {
	opts = normalizeIdempotencyOptions(opts)

	return func(c *gin.Context) {
		// 客户端需在 Header 中传递唯一的幂等键标识
		rawKey := c.GetHeader(opts.Header)
		if rawKey == "" || !methodAllowed(opts.Methods, c.Request.Method) {
			c.Next()
			return
		}

		ctx := c.Request.Context()
		key := buildIdempotencyKey(ctx, rawKey, c.Request.Method, c.Request.URL.Path)

		// 1. 尝试判定并锁定当前请求。
		isFirst, savedResp, err := manager.TryStart(ctx, key, opts.LockTTL)
		if err != nil {
			if errors.Is(err, idempotency.ErrInProgress) {
				logging.Warn(ctx, "idempotency in progress", "key", key)
				response.ErrorWithStatus(c, http.StatusConflict, "request is being processed, please do not repeat", "idempotency active")
				c.Abort()
				return
			}
			logging.Error(ctx, "idempotency manager failure, fallback to passthrough", "key", key, "error", err)
			c.Next()
			return
		}

		// 2. 缓存命中：如果请求之前已成功处理，直接还原并返回之前的响应快照。
		if !isFirst && savedResp != nil {
			for k, v := range savedResp.Header {
				if v != "" {
					c.Header(k, v)
				}
			}
			c.Header(IdempotencyCacheHeader, "HIT")
			c.Data(savedResp.StatusCode, "application/json; charset=utf-8", []byte(savedResp.Body))
			c.Abort()
			return
		}

		// 3. 首次处理：拦截 Writer 缓冲区以捕获业务响应。
		w := &responseBodyWriter{body: &bytes.Buffer{}, ResponseWriter: c.Writer}
		c.Writer = w

		c.Next()

		// 4. 后置处理：只对成功的业务响应进行幂等快照持久化。
		if c.Writer.Status() >= http.StatusOK && c.Writer.Status() < http.StatusMultipleChoices {
			respSnapshot := &idempotency.Response{
				StatusCode: c.Writer.Status(),
				Body:       w.body.String(),
				Header:     flattenHeader(c.Writer.Header()),
			}
			if err := manager.Finish(ctx, key, respSnapshot, opts.CacheTTL); err != nil {
				logging.Error(ctx, "failed to persist idempotency snapshot", "key", key, "error", err)
			}
		} else {
			// 若业务处理失败（如 4xx/5xx），则主动释放幂等锁，允许修正参数后重试。
			if err := manager.Delete(ctx, key); err != nil {
				logging.Error(ctx, "failed to release idempotency lock on failure", "key", key, "error", err)
			}
		}
	}
}

func normalizeIdempotencyOptions(opts IdempotencyOptions) IdempotencyOptions {
	if opts.Header == "" {
		opts.Header = IdempotencyKeyHeader
	}
	if opts.LockTTL <= 0 {
		opts.LockTTL = 5 * time.Minute
	}
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = 24 * time.Hour
	}
	return opts
}

func methodAllowed(methods []string, method string) bool {
	if len(methods) == 0 {
		return true
	}
	for _, m := range methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

func buildIdempotencyKey(ctx context.Context, key, method, path string) string {
	key = strings.TrimSpace(key)
	tenantID := contextx.GetTenantID(ctx)
	userID := contextx.GetUserID(ctx)

	if tenantID == "" && userID == "" {
		return method + ":" + path + ":" + key
	}
	if tenantID == "" {
		tenantID = "unknown"
	}
	if userID == "" {
		userID = "unknown"
	}

	return tenantID + ":" + userID + ":" + method + ":" + path + ":" + key
}

func flattenHeader(header http.Header) map[string]string {
	if len(header) == 0 {
		return map[string]string{}
	}

	result := make(map[string]string, len(header))
	for k, vals := range header {
		if len(vals) == 0 {
			continue
		}
		result[k] = strings.Join(vals, ",")
	}
	return result
}
