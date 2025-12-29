package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/idempotency"
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

// IdempotencyMiddleware 创建一个幂等中间件。
// manager: 幂等管理实现。
// ttl: 成功结果在 Redis 中的保留时间。
func IdempotencyMiddleware(manager idempotency.Manager, ttl time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-Idempotency-Key")
		if key == "" {
			c.Next()
			return
		}

		// 1. 尝试锁定请求
		isFirst, savedResp, err := manager.TryStart(c.Request.Context(), key, 5*time.Minute) // 默认处理中锁 5 分钟
		if err != nil {
			if err == idempotency.ErrInProgress {
				c.JSON(http.StatusConflict, gin.H{"error": "request is being processed, please do not repeat"})
				c.Abort()
				return
			}
			slog.Error("idempotency manager error", "error", err)
			c.Next() // 降级处理：管理系统故障时允许请求通过
			return
		}

		// 2. 如果之前已经成功处理过，直接返回缓存的结果
		if !isFirst && savedResp != nil {
			for k, v := range savedResp.Header {
				c.Header(k, v)
			}
			c.Header("X-Idempotency-Cache", "HIT")
			c.Data(savedResp.StatusCode, "application/json; charset=utf-8", []byte(savedResp.Body))
			c.Abort()
			return
		}

		// 3. 第一次处理，捕获响应
		w := &responseBodyWriter{body: &bytes.Buffer{}, ResponseWriter: c.Writer}
		c.Writer = w

		c.Next()

		// 4. 只针对成功的响应进行幂等存储 (2xx 状态码)
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			respSnapshot := &idempotency.Response{
				StatusCode: c.Writer.Status(),
				Body:       w.body.String(),
				Header:     make(map[string]string),
			}
			// 这里可以根据需要过滤想要缓存的 Header
			if err := manager.Finish(c.Request.Context(), key, respSnapshot, ttl); err != nil {
				slog.Error("failed to finish idempotency", "key", key, "error", err)
			}
		} else {
			// 如果处理失败，释放锁，允许客户端修改参数后重试
			_ = manager.Delete(c.Request.Context(), key)
		}
	}
}
