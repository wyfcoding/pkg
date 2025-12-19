package server

import (
	"log/slog" // 导入结构化日志库。

	"github.com/wyfcoding/pkg/middleware" // 导入项目内定义的中间件。

	"github.com/gin-gonic/gin" // 导入Gin Web框架。
)

// NewDefaultGinEngine 创建一个新的 Gin 引擎实例，并配置常用的默认中间件。
// 它还允许在创建时传入额外的自定义中间件。
// logger: 用于请求日志的日志记录器。
// middlewares: 可选的自定义 Gin 中间件。
// 返回一个配置好的 `*gin.Engine` 实例。
func NewDefaultGinEngine(logger *slog.Logger, middlewares ...gin.HandlerFunc) *gin.Engine {
	engine := gin.New() // 创建一个不带任何中间件的Gin引擎实例。

	// 应用默认中间件：
	// gin.Recovery()：从任何panic中恢复，并返回500错误。
	engine.Use(gin.Recovery())
	// middleware.Logger(logger)：项目自定义的请求日志中间件，用于记录HTTP请求信息。
	engine.Use(middleware.Logger(logger))

	// 应用外部传入的自定义中间件。
	engine.Use(middlewares...)

	return engine
}
