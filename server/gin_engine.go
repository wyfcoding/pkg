// Package server 提供了启动和管理gRPC和HTTP服务器的封装。
// 生成摘要:
// 1) Gin 引擎不再内置默认中间件，统一由调用方决定顺序与集合。
// 假设:
// 1) 调用方会显式注入必要的 Recovery/Logger 等治理中间件。
package server

import (
	"github.com/gin-gonic/gin" // 导入Gin Web框架。
)

// NewDefaultGinEngine 创建一个新的 Gin 引擎实例。
// 由调用方负责决定中间件顺序与集合，以满足不同服务的治理策略。
func NewDefaultGinEngine(middlewares ...gin.HandlerFunc) *gin.Engine {
	// 创建一个不带任何默认配置的干净引擎，以便精准控制中间件。
	engine := gin.New()

	// 注册自定义中间件（如限流、熔断、鉴权、追踪等）。
	engine.Use(middlewares...)

	return engine
}
