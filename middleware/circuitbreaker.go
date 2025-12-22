// Package middleware 提供了Gin和gRPC的通用中间件。
package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sony/gobreaker"
)

// CircuitBreaker 创建并返回一个Gin中间件，用于实现熔断功能。
// 熔断器可以防止系统在面对故障服务时持续发送请求，从而避免雪崩效应。
//
// 熔断器的工作状态包括：
//   - Closed（关闭）：正常状态，所有请求都通过。
//   - Open（开启）：当失败率达到阈值时，熔断器打开，所有请求直接失败，不再尝试调用后端服务。
//   - Half-Open（半开）：经过一段超时时间后，熔断器进入半开状态，允许少量请求通过以探测后端服务是否恢复。
func CircuitBreaker() gin.HandlerFunc {
	// 配置熔断器的设置

	st := gobreaker.Settings{
		Name:        "HTTP-Circuit-Breaker", // 熔断器名称，用于指标和日志
		MaxRequests: 0,                      // 在Half-Open状态下，允许通过的请求数，0表示不限制（但仍受限于总超时）
		Interval:    60 * time.Second,       // 熔断器在Closed状态下，统计故障率的周期
		Timeout:     60 * time.Second,       // 熔断器从Open状态自动转换为Half-Open状态的时间
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// ReadyToTrip 函数定义了从Closed状态切换到Open状态的条件。
			// counts 包含了在当前统计周期内的请求总数、成功数和失败数。
			// 这里设定：如果总请求数达到10次，并且失败率达到60%，则触发熔断。
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 10 && failureRatio >= 0.6
		},
		// OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
		// 	// 可选：状态改变时触发的回调函数，用于记录日志或发送警报
		// 	fmt.Printf("Circuit Breaker '%s' changed from %s to %s\n", name, from, to)
		// },
	}
	// 创建一个新的熔断器实例
	cb := gobreaker.NewCircuitBreaker(st)

	return func(c *gin.Context) {
		// 使用熔断器的Execute方法来包装实际的请求处理逻辑
		// 如果熔断器处于Open状态，Execute会立即返回 gobreaker.ErrOpenState
		// 如果熔断器允许请求通过，它会执行传入的匿名函数
		_, err := cb.Execute(func() (any, error) {
			// 在熔断器内部执行Gin请求链的其余部分
			c.Next()
			// 根据Gin处理后的HTTP状态码判断是否为失败。
			// 如果状态码是5xx系列（服务器内部错误），则认为是失败，返回一个错误给熔断器。
			if c.Writer.Status() >= 500 {
				// 返回一个错误，gobreaker会将其计为失败
				return nil, http.ErrHandlerTimeout // 示例：将5xx视为超时错误
			}
			return nil, nil // 否则视为成功
		})

		if err != nil {
			// 如果熔断器处于Open状态，Execute会返回 gobreaker.ErrOpenState
			if err == gobreaker.ErrOpenState {
				// 熔断器开启，返回服务不可用状态
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "circuit breaker open"})
				return
			}
			// 其他错误（例如，匿名函数内部发生的错误）会由 c.Next() 负责处理，
			// 或者在Gin的错误处理链中进行。这里不需额外处理。
		}
	}
}
