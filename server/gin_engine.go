package server

import (
	"github.com/wyfcoding/pkg/middleware" // 导入项目内定义的中间件。

	"github.com/gin-gonic/gin" // 导入Gin Web框架。
)

// NewDefaultGinEngine 创建一个新的 Gin 引擎实例，并预置工业级标准中间件。
// 默认包含：Recovery (异常恢复)、RequestLogger (结构化访问日志)。
// 同时支持在初始化时注入额外的业务自定义中间件。
func NewDefaultGinEngine(middlewares ...gin.HandlerFunc) *gin.Engine {
	// 创建一个不带任何默认配置的干净引擎，以便精准控制中间件。
	engine := gin.New()

	// 1. 注册核心中间件：确保任何 Panic 都能被记录并转化为 500 响应。
	engine.Use(middleware.Recovery())

	// 2. 注册日志中间件：输出符合 trace 规范的访问日志。
	engine.Use(middleware.RequestLogger())

	// 3. 注册额外的扩展中间件（如限流、熔断、鉴权等）。
	engine.Use(middlewares...)

	return engine
}
