package pagination

import "math"

// 常量定义，集中管理分页约束
const (
	DefaultPage     = 1
	DefaultPageSize = 10
	MaxPageSize     = 500
)

// Request 分页请求通用结构
type Request struct {
	Page     int `json:"page" form:"page"`           // 当前页码 (从1开始)
	PageSize int `json:"page_size" form:"page_size"` // 每页数量
}

// NewRequest 创建并校验分页请求
func NewRequest(page, pageSize int) *Request {
	r := &Request{Page: page, PageSize: pageSize}
	r.Validate()
	return r
}

// Validate 强制执行安全约束和默认值
func (r *Request) Validate() {
	if r.Page <= 0 {
		r.Page = DefaultPage
	}
	if r.PageSize <= 0 {
		r.PageSize = DefaultPageSize
	}
	if r.PageSize > MaxPageSize {
		r.PageSize = MaxPageSize
	}
}

// Offset 返回 GORM 所需的偏移量
func (r *Request) Offset() int {
	return (r.Page - 1) * r.PageSize
}

// Limit 返回 GORM 所需的限制数
func (r *Request) Limit() int {
	return r.PageSize
}

// Result 泛型分页响应结果，T 为业务实体类型
type Result[T any] struct {
	Total      int64 `json:"total"`       // 总记录数
	Page       int   `json:"page"`        // 当前页码
	PageSize   int   `json:"page_size"`   // 每页数量
	TotalPages int   `json:"total_pages"` // 总页数
	HasMore    bool  `json:"has_more"`    // 是否有下一页
	Data       []T   `json:"list"`        // 数据列表
}

// NewResult 创建一个类型安全的分页结果
func NewResult[T any](total int64, req *Request, items []T) *Result[T] {
	if items == nil {
		items = make([]T, 0)
	}

	// 计算总页数
	totalPages := int(math.Ceil(float64(total) / float64(req.PageSize)))
	if total == 0 {
		totalPages = 0
	}

	return &Result[T]{
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
		HasMore:    req.Page < totalPages,
		Data:       items,
	}
}

// --- Cursor (游标分页支持，用于深度分页优化，避免 OFFSET 性能问题) ---

// CursorRequest 定义了基于游标的分页请求结构。
// 适用于无限滚动或高频数据流场景。
type CursorRequest struct {
	LastID   uint64 `json:"last_id" form:"last_id"`     // 上一页返回数据中最后一条记录的唯一标识 (ID)
	PageSize int    `json:"page_size" form:"page_size"` // 请求抓取的数据条数
}

// CursorResult 定义了游标分页的响应结果。
type CursorResult[T any] struct {
	NextID uint64 `json:"next_id"` // 指向下一页起始位置的 ID，若为 0 则表示后续无更多数据
	Data   []T    `json:"list"`    // 本次返回的业务实体列表
}
