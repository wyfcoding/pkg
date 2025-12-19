package response

import (
	"net/http" // 导入HTTP状态码。

	"github.com/gin-gonic/gin" // 导入Gin Web框架。
)

// Success 发送一个标准的成功响应。
// HTTP状态码为 200 OK，业务码为 0，消息为 "success"。
// c: Gin上下文。
// data: 响应数据，可以是任何可JSON序列化的类型。
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,         // 业务成功码。
		"msg":  "success", // 成功消息。
		"data": data,      // 实际返回的数据。
	})
}

// SuccessWithStatus 发送一个带有指定HTTP状态码和消息的成功响应。
// c: Gin上下文。
// status: HTTP状态码（例如：http.StatusOK, http.StatusCreated）。
// msg: 响应消息。
// data: 响应数据。
func SuccessWithStatus(c *gin.Context, status int, msg string, data interface{}) {
	c.JSON(status, gin.H{
		"code": 0,    // 业务成功码。
		"msg":  msg,  // 自定义成功消息。
		"data": data, // 实际返回的数据。
	})
}

// SuccessWithMessage 发送一个带有自定义消息的成功响应。
// HTTP状态码为 200 OK，业务码为 0。
// c: Gin上下文。
// msg: 响应消息。
// data: 响应数据。
func SuccessWithMessage(c *gin.Context, msg string, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,    // 业务成功码。
		"msg":  msg,  // 自定义成功消息。
		"data": data, // 实际返回的数据。
	})
}

// Error 发送一个通用的错误响应。
// HTTP状态码为 500 Internal Server Error，业务码为 500。
// c: Gin上下文。
// err: 错误信息。
func Error(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"code": 500,         // 业务错误码。
		"msg":  err.Error(), // 错误消息。
	})
}

// ErrorWithStatus 发送一个带有指定HTTP状态码、消息和详情的错误响应。
// c: Gin上下文。
// status: HTTP状态码（例如：http.StatusBadRequest, http.StatusUnauthorized）。
// msg: 错误消息。
// detail: 错误的详细描述。
func ErrorWithStatus(c *gin.Context, status int, msg string, detail string) {
	c.JSON(status, gin.H{
		"code":   status, // 业务错误码（与HTTP状态码相同）。
		"msg":    msg,    // 错误消息。
		"detail": detail, // 错误详情。
	})
}

// ErrorWithCode 发送一个带有指定业务错误码和消息的错误响应。
// HTTP状态码与业务错误码相同。
// c: Gin上下文。
// code: 业务错误码。
// msg: 错误消息。
func ErrorWithCode(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{
		"code": code, // 业务错误码。
		"msg":  msg,  // 错误消息。
	})
}

// SuccessWithPagination 发送一个包含分页信息的成功响应。
// HTTP状态码为 200 OK，业务码为 0。
// c: Gin上下文。
// data: 当前页的数据列表。
// total: 总记录数。
// page: 当前页码。
// size: 每页数量。
func SuccessWithPagination(c *gin.Context, data interface{}, total int64, page, size int32) {
	c.JSON(http.StatusOK, gin.H{
		"code":  0,         // 业务成功码。
		"msg":   "success", // 成功消息。
		"data":  data,      // 当前页的数据列表。
		"total": total,     // 总记录数。
		"page":  page,      // 当前页码。
		"size":  size,      // 每页数量。
	})
}

// BadRequest 发送一个 HTTP 400 Bad Request 错误响应。
// c: Gin上下文。
// msg: 错误消息。
func BadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"code": 400, // 业务错误码。
		"msg":  msg, // 错误消息。
	})
}

// Unauthorized 发送一个 HTTP 401 Unauthorized 错误响应。
// c: Gin上下文。
// msg: 错误消息。
func Unauthorized(c *gin.Context, msg string) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"code": 401, // 业务错误码。
		"msg":  msg, // 错误消息。
	})
}

// NotFound 发送一个 HTTP 404 Not Found 错误响应。
// c: Gin上下文。
// msg: 错误消息。
func NotFound(c *gin.Context, msg string) {
	c.JSON(http.StatusNotFound, gin.H{
		"code": 404, // 业务错误码。
		"msg":  msg, // 错误消息。
	})
}

// InternalError 发送一个 HTTP 500 Internal Server Error 错误响应。
// c: Gin上下文。
// msg: 错误消息。
func InternalError(c *gin.Context, msg string) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"code": 500, // 业务错误码。
		"msg":  msg, // 错误消息。
	})
}
