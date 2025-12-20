package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

// Logger 返回一个请求日志中间件。
// 该中间件用于记录每个HTTP请求的关键信息，如请求方法、路径、状态码、耗时等。
func Logger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 记录请求开始时间。
		start := time.Now()
		// 获取请求路径和查询参数。
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 调用请求链中的下一个处理程序。
		c.Next()

		// 计算请求处理耗时。
		cost := time.Since(start)

		// 使用结构化日志记录请求信息。
		logger.InfoContext(c.Request.Context(), "Request",
			"status", c.Writer.Status(), // HTTP响应状态码。
			"method", c.Request.Method, // HTTP请求方法。
			"path", path, // 请求路径。
			"query", query, // 请求的原始查询字符串。
			"ip", c.ClientIP(), // 客户端IP地址。
			"user-agent", c.Request.UserAgent(), // 客户端User-Agent。
			"errors", c.Errors.ByType(gin.ErrorTypePrivate).String(), // Gin内部捕获的错误信息。
			"cost", cost, // 请求处理总耗时。
		)
	}
}

// GRPCLoggingInterceptor 返回一个新的用于日志记录的一元服务器拦截器。
func GRPCLoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		// 调用处理程序
		resp, err := handler(ctx, req)
		cost := time.Since(start)

		// Log request
		// 假设我们为 gRPC 中间件使用默认 logger，或者我们可以传递一个。
		// 目前使用我们在 InitLogger 中设置的 slog.Default()。
		log := slog.Default()

		if err != nil {
			log.Error("GRPC Request",
				"method", info.FullMethod,
				"error", err,
				"cost", cost,
			)
		} else {
			log.Info("GRPC Request",
				"method", info.FullMethod,
				"cost", cost,
			)
		}

		return resp, err
	}
}
