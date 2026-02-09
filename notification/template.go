// Package notification 提供模板渲染实现。
// 生成摘要：
// 1) 支持 Go text/template 语法
// 2) 支持从文件/数据库/内存加载模板
// 假设：
// - 模板变量使用 {{.FieldName}} 格式
// - 支持条件判断和循环
package notification

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"text/template"
)

// Template 表示通知模板。
type Template struct {
	// ID 模板唯一标识。
	ID string `json:"id"`
	// Name 模板名称。
	Name string `json:"name"`
	// Channel 适用渠道。
	Channel Channel `json:"channel"`
	// Subject 标题模板（用于邮件/Push）。
	Subject string `json:"subject,omitempty"`
	// Content 内容模板。
	Content string `json:"content"`
	// Variables 变量定义（用于校验和文档）。
	Variables []TemplateVariable `json:"variables,omitempty"`
	// Enabled 是否启用。
	Enabled bool `json:"enabled"`
}

// TemplateVariable 模板变量定义。
type TemplateVariable struct {
	// Name 变量名。
	Name string `json:"name"`
	// Required 是否必需。
	Required bool `json:"required"`
	// Default 默认值。
	Default string `json:"default,omitempty"`
	// Description 变量说明。
	Description string `json:"description,omitempty"`
}

// TemplateStore 定义模板存储接口。
type TemplateStore interface {
	// Get 获取模板。
	Get(ctx context.Context, id string) (*Template, error)
	// List 列出所有模板。
	List(ctx context.Context, channel Channel) ([]*Template, error)
	// Save 保存模板。
	Save(ctx context.Context, tmpl *Template) error
	// Delete 删除模板。
	Delete(ctx context.Context, id string) error
}

// MemoryTemplateStore 内存模板存储实现。
type MemoryTemplateStore struct {
	templates map[string]*Template
	mu        sync.RWMutex
}

// NewMemoryTemplateStore 创建内存模板存储。
func NewMemoryTemplateStore() *MemoryTemplateStore {
	return &MemoryTemplateStore{
		templates: make(map[string]*Template),
	}
}

// Get 获取模板。
func (s *MemoryTemplateStore) Get(ctx context.Context, id string) (*Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tmpl, ok := s.templates[id]
	if !ok {
		return nil, fmt.Errorf("template not found: %s", id)
	}
	return tmpl, nil
}

// List 列出所有模板。
func (s *MemoryTemplateStore) List(ctx context.Context, channel Channel) ([]*Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Template
	for _, tmpl := range s.templates {
		if channel == "" || tmpl.Channel == channel {
			result = append(result, tmpl)
		}
	}
	return result, nil
}

// Save 保存模板。
func (s *MemoryTemplateStore) Save(ctx context.Context, tmpl *Template) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.templates[tmpl.ID] = tmpl
	return nil
}

// Delete 删除模板。
func (s *MemoryTemplateStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.templates, id)
	return nil
}

// DefaultTemplateRenderer 默认模板渲染器实现。
type DefaultTemplateRenderer struct {
	store  TemplateStore
	cache  map[string]*compiledTemplate
	mu     sync.RWMutex
	logger *slog.Logger
}

// compiledTemplate 编译后的模板缓存。
type compiledTemplate struct {
	subject *template.Template
	content *template.Template
}

// NewDefaultTemplateRenderer 创建默认模板渲染器。
func NewDefaultTemplateRenderer(store TemplateStore, logger *slog.Logger) *DefaultTemplateRenderer {
	if logger == nil {
		logger = slog.Default()
	}
	return &DefaultTemplateRenderer{
		store:  store,
		cache:  make(map[string]*compiledTemplate),
		logger: logger,
	}
}

// Render 渲染模板。
func (r *DefaultTemplateRenderer) Render(ctx context.Context, templateID string, variables map[string]string) (subject, content string, err error) {
	// 尝试从缓存获取编译后的模板
	compiled, err := r.getCompiled(ctx, templateID)
	if err != nil {
		return "", "", err
	}

	// 准备模板数据
	data := make(map[string]any)
	for k, v := range variables {
		data[k] = v
	}

	// 渲染标题
	if compiled.subject != nil {
		var subjectBuf bytes.Buffer
		if err := compiled.subject.Execute(&subjectBuf, data); err != nil {
			r.logger.ErrorContext(ctx, "failed to render subject template",
				"template_id", templateID,
				"error", err)
			return "", "", fmt.Errorf("failed to render subject: %w", err)
		}
		subject = subjectBuf.String()
	}

	// 渲染内容
	var contentBuf bytes.Buffer
	if err := compiled.content.Execute(&contentBuf, data); err != nil {
		r.logger.ErrorContext(ctx, "failed to render content template",
			"template_id", templateID,
			"error", err)
		return "", "", fmt.Errorf("failed to render content: %w", err)
	}
	content = contentBuf.String()

	return subject, content, nil
}

// getCompiled 获取或编译模板。
func (r *DefaultTemplateRenderer) getCompiled(ctx context.Context, templateID string) (*compiledTemplate, error) {
	if r.store == nil {
		return nil, fmt.Errorf("template store is nil")
	}
	// 先尝试从缓存获取
	r.mu.RLock()
	if compiled, ok := r.cache[templateID]; ok {
		r.mu.RUnlock()
		return compiled, nil
	}
	r.mu.RUnlock()

	// 从存储获取模板
	tmpl, err := r.store.Get(ctx, templateID)
	if err != nil {
		return nil, err
	}

	if !tmpl.Enabled {
		return nil, fmt.Errorf("template is disabled: %s", templateID)
	}

	// 编译模板
	compiled := &compiledTemplate{}

	if tmpl.Subject != "" {
		subjectTmpl, err := template.New("subject").Parse(tmpl.Subject)
		if err != nil {
			return nil, fmt.Errorf("failed to parse subject template: %w", err)
		}
		compiled.subject = subjectTmpl
	}

	contentTmpl, err := template.New("content").Parse(tmpl.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse content template: %w", err)
	}
	compiled.content = contentTmpl

	// 缓存编译后的模板
	r.mu.Lock()
	r.cache[templateID] = compiled
	r.mu.Unlock()

	return compiled, nil
}

// InvalidateCache 清除模板缓存。
func (r *DefaultTemplateRenderer) InvalidateCache(templateID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if templateID == "" {
		// 清除所有缓存
		r.cache = make(map[string]*compiledTemplate)
	} else {
		delete(r.cache, templateID)
	}
}

// ValidateTemplate 校验模板语法。
func ValidateTemplate(subject, content string) error {
	if subject != "" {
		if _, err := template.New("subject").Parse(subject); err != nil {
			return fmt.Errorf("invalid subject template: %w", err)
		}
	}

	if content == "" {
		return fmt.Errorf("content template is required")
	}

	if _, err := template.New("content").Parse(content); err != nil {
		return fmt.Errorf("invalid content template: %w", err)
	}

	return nil
}

// CommonTemplates 预定义的常用模板。
var CommonTemplates = map[string]*Template{
	"verification_code": {
		ID:      "verification_code",
		Name:    "验证码",
		Channel: ChannelSMS,
		Content: "【{{.AppName}}】您的验证码是{{.Code}}，有效期{{.ExpireMinutes}}分钟。请勿泄露给他人。",
		Variables: []TemplateVariable{
			{Name: "AppName", Required: true, Description: "应用名称"},
			{Name: "Code", Required: true, Description: "验证码"},
			{Name: "ExpireMinutes", Required: true, Description: "有效期（分钟）"},
		},
		Enabled: true,
	},
	"order_shipped": {
		ID:      "order_shipped",
		Name:    "订单发货通知",
		Channel: ChannelSMS,
		Content: "【{{.AppName}}】您的订单{{.OrderNo}}已发货，快递公司：{{.Carrier}}，快递单号：{{.TrackingNo}}。",
		Variables: []TemplateVariable{
			{Name: "AppName", Required: true, Description: "应用名称"},
			{Name: "OrderNo", Required: true, Description: "订单号"},
			{Name: "Carrier", Required: true, Description: "快递公司"},
			{Name: "TrackingNo", Required: true, Description: "快递单号"},
		},
		Enabled: true,
	},
	"payment_success": {
		ID:      "payment_success",
		Name:    "支付成功通知",
		Channel: ChannelPush,
		Subject: "支付成功",
		Content: "您已成功支付订单{{.OrderNo}}，金额{{.Amount}}元。",
		Variables: []TemplateVariable{
			{Name: "OrderNo", Required: true, Description: "订单号"},
			{Name: "Amount", Required: true, Description: "支付金额"},
		},
		Enabled: true,
	},
	"welcome_email": {
		ID:      "welcome_email",
		Name:    "欢迎邮件",
		Channel: ChannelEmail,
		Subject: "欢迎加入{{.AppName}}",
		Content: `尊敬的{{.Username}}：

感谢您注册成为{{.AppName}}会员！

即日起，您可以享受以下会员权益：
- 专属优惠券
- 积分累计
- 会员专享价

祝您购物愉快！

{{.AppName}}团队`,
		Variables: []TemplateVariable{
			{Name: "AppName", Required: true, Description: "应用名称"},
			{Name: "Username", Required: true, Description: "用户名"},
		},
		Enabled: true,
	},
}

// LoadCommonTemplates 加载预定义模板到存储。
func LoadCommonTemplates(ctx context.Context, store TemplateStore) error {
	for _, tmpl := range CommonTemplates {
		if err := store.Save(ctx, tmpl); err != nil {
			return fmt.Errorf("failed to load template %s: %w", tmpl.ID, err)
		}
	}
	return nil
}
