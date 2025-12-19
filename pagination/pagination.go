package pagination

// Page 结构体定义了分页请求的参数。
// 它通常作为HTTP请求的查询参数或请求体的一部分，用于指定获取数据的页码和每页数量。
type Page struct {
	PageNum  int `json:"page_num" form:"page_num"`   // 页码，通常从1开始计数。
	PageSize int `json:"page_size" form:"page_size"` // 每页包含的记录数量。
}

// Validate 验证分页参数的合法性，并设置默认值。
// 它确保页码和每页数量是有效的，并限制每页数量的最大值。
func (p *Page) Validate() {
	if p.PageNum <= 0 {
		p.PageNum = 1 // 页码不能小于等于0，默认为第一页。
	}
	if p.PageSize <= 0 {
		p.PageSize = 10 // 每页数量不能小于等于0，默认为10条。
	}
	if p.PageSize > 100 {
		p.PageSize = 100 // 每页数量不能超过100，防止单次请求数据量过大。
	}
}

// Offset 计算数据库查询的偏移量（offset）。
// 偏移量 = (页码 - 1) * 每页数量。
// 例如，第一页的偏移量为0，第二页的偏移量为 PageSize。
func (p *Page) Offset() int {
	return (p.PageNum - 1) * p.PageSize
}

// Limit 返回数据库查询的限制数量（limit）。
// 即每页需要返回的记录数量。
func (p *Page) Limit() int {
	return p.PageSize
}

// PageResult 结构体定义了分页查询的返回结果。
// 它包含了查询到的总记录数、当前页码、每页数量以及实际的数据列表。
type PageResult struct {
	Total    int64       `json:"total"`     // 符合查询条件的总记录数。
	PageNum  int         `json:"page_num"`  // 当前页码。
	PageSize int         `json:"page_size"` // 每页数量。
	Data     interface{} `json:"data"`      // 当前页的数据列表（可以是任意类型）。
}

// NewPageResult 创建并返回一个新的 PageResult 实例。
// total: 符合查询条件的总记录数。
// page: 包含当前页码和每页数量的 Page 结构体。
// data: 当前页的数据列表。
func NewPageResult(total int64, page *Page, data interface{}) *PageResult {
	return &PageResult{
		Total:    total,
		PageNum:  page.PageNum,
		PageSize: page.PageSize,
		Data:     data,
	}
}

// TotalPages 计算总页数。
func (r *PageResult) TotalPages() int {
	if r.PageSize == 0 {
		return 0 // 避免除以零。
	}
	// 计算总页数，向上取整。
	pages := int(r.Total) / r.PageSize
	if int(r.Total)%r.PageSize > 0 {
		pages++
	}
	return pages
}

// HasNext 检查是否存在下一页。
func (r *PageResult) HasNext() bool {
	return r.PageNum < r.TotalPages()
}

// HasPrev 检查是否存在上一页。
func (r *PageResult) HasPrev() bool {
	return r.PageNum > 1
}
