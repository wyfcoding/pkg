// Package middleware 提供了通用的 Gin 中间件。
package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/response"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Recovery 返回一个自定义的 Gin Panic 恢复中间件。
// 它会将堆栈信息记录到 slog 中，并返回 500 状态码。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 检查是否为断开的连接。
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if strings.Contains(strings.ToLower(se.Error()), "broken pipe") ||
							strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				httpRequest, _ := httputil.DumpRequest(c.Request, false)
				if brokenPipe {
					logging.Error(c.Request.Context(), "broken pipe recovered from panic",
						"error", err,
						"request", string(httpRequest),
					)
					// 断开的连接无法写状态码。
					c.Error(err.(error))
					c.Abort()
					return
				}

				logging.Error(c.Request.Context(), "recovered from panic",
					"error", err,
					"stack", string(debug.Stack()),
					"path", c.Request.URL.Path,
				)

				// 返回符合项目标准的 500 内部服务器错误响应。
				response.ErrorWithStatus(c, http.StatusInternalServerError,
					"Internal Server Error",
					fmt.Sprintf("A panic occurred: %v", err))

				c.Abort()
			}
		}()

		c.Next()
	}
}

// GRPCRecovery 返回一个 gRPC 一元拦截器，用于捕获 Panic 并记录日志。
func GRPCRecovery() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				logging.Error(ctx, "gRPC recovered from panic",
					"error", r,
					"method", info.FullMethod,
					"stack", string(debug.Stack()),
				)
				err = status.Errorf(codes.Internal, "panic recovered: %v", r)
			}
		}()

		return handler(ctx, req)
	}
}
