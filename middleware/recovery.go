package middleware

import (
	"context"
	"log/slog"      // 导入结构化日志库。
	"net/http"      // 导入HTTP状态码。
	"runtime/debug" // 导入用于获取堆栈信息的debug包。

	"github.com/wyfcoding/pkg/response" // 导入项目内定义的响应处理工具。

	"github.com/gin-gonic/gin" // 导入Gin Web框架。
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Recovery 是一个Gin中间件，用于从处理请求过程中发生的panic中恢复。
// 它会捕获panic，记录详细的错误信息（包括堆栈跟踪），并向客户端返回一个统一的500内部服务器错误响应，
// 从而防止服务器崩溃，提高服务的健壮性。
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// defer 语句确保在函数返回之前执行匿名函数，即使发生panic。
		defer func() {
			if err := recover(); err != nil { // recover() 尝试从panic中恢复。
				// 记录panic的详细信息，包括错误内容和完整的堆栈跟踪。
				logger.Error("Panic recovered",
					"error", err, // panic的具体错误。
					"stack", string(debug.Stack()), // 记录发生panic时的完整堆栈信息。
				)

				// 向客户端返回一个HTTP 500内部服务器错误响应。
				// 使用项目统一的响应处理函数来构建错误响应。
				response.ErrorWithStatus(c, http.StatusInternalServerError, "Internal Server Error", "An unexpected error occurred")
				c.Abort() // 中止请求链，不再执行后续处理器，确保错误响应被发送。
			}
		}()
		// 继续执行请求链中的下一个中间件或处理器。
		c.Next()
	}
}

// GRPCRecoveryInterceptor 返回一个新的用于恐慌恢复的一元服务器拦截器。
func GRPCRecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				// 记录恐慌日志
				slog.Error("GRPC Panic recovered",
					"method", info.FullMethod,
					"error", r,
					"stack", string(debug.Stack()),
				)
				// Return Internal error
				err = status.Errorf(codes.Internal, "Internal server error: %v", r)
			}
		}()
		return handler(ctx, req)
	}
}
