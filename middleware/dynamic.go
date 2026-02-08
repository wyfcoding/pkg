package middleware

import (
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// DynamicHandler 提供可热更新的 Gin 中间件包装器。
type DynamicHandler struct {
	value atomic.Value
}

// NewDynamicHandler 创建动态中间件封装器。
func NewDynamicHandler(initial gin.HandlerFunc) *DynamicHandler {
	handler := &DynamicHandler{}
	if initial != nil {
		handler.value.Store(initial)
	}
	return handler
}

// Update 更新当前中间件实现。
func (d *DynamicHandler) Update(h gin.HandlerFunc) {
	if d == nil {
		return
	}
	d.value.Store(h)
}

// Handler 返回可注册到 Gin 的动态中间件。
func (d *DynamicHandler) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if d == nil {
			c.Next()
			return
		}
		h := d.load()
		if h == nil {
			c.Next()
			return
		}
		h(c)
	}
}

func (d *DynamicHandler) load() gin.HandlerFunc {
	if d == nil {
		return nil
	}
	val := d.value.Load()
	if val == nil {
		return nil
	}
	return val.(gin.HandlerFunc)
}
