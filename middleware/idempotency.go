package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/idempotency"
	"github.com/wyfcoding/pkg/response"
)

// responseBodyWriter 用于捕获 Gin 的响应内容
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
	return func(c *gin.Context) {
		// 客户端需在 Header 中传递唯一的幂等键标识
		key := c.GetHeader("X-Idempotency-Key")
		if key == "" {
			c.Next()
			return
		}

		ctx := c.Request.Context()

		// 1. 尝试判定并锁定当前请求。
		// 锁定 TTL 默认为 5 分钟，防止由于业务异常未调用 Finish 导致的死锁。
		isFirst, savedResp, err := manager.TryStart(ctx, key, 5*time.Minute)
		if err != nil {
			if err == idempotency.ErrInProgress {
				response.ErrorWithStatus(c, http.StatusConflict, "request is being processed, please do not repeat", "idempotency active")
				c.Abort()
				return
			}
			slog.ErrorContext(ctx, "idempotency manager failure, fallback to passthrough", "key", key, "error", err)
			c.Next()
			return
		}

		// 2. 缓存命中：如果请求之前已成功处理，直接还原并返回之前的响应快照。
		if !isFirst && savedResp != nil {
			for k, v := range savedResp.Header {
				c.Header(k, v)
			}
			c.Header("X-Idempotency-Cache", "HIT")
			c.Data(savedResp.StatusCode, "application/json; charset=utf-8", []byte(savedResp.Body))
			c.Abort()
			return
		}

		// 3. 首次处理：拦截 Writer 缓冲区以捕获业务响应。
		w := &responseBodyWriter{body: &bytes.Buffer{}, ResponseWriter: c.Writer}
		c.Writer = w

		c.Next()

		// 4. 后置处理：只对成功的业务响应进行幂等快照持久化。
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			respSnapshot := &idempotency.Response{
				StatusCode: c.Writer.Status(),
				Body:       w.body.String(),
				Header:     make(map[string]string),
			}
			if err := manager.Finish(ctx, key, respSnapshot, ttl); err != nil {
				slog.ErrorContext(ctx, "failed to persist idempotency snapshot", "key", key, "error", err)
			}
		} else {
			// 若业务处理失败（如 4xx/5xx），则主动释放幂等锁，允许修正参数后重试。
			if err := manager.Delete(ctx, key); err != nil {
				slog.ErrorContext(ctx, "failed to release idempotency lock on failure", "key", key, "error", err)
			}
		}
	}
}
