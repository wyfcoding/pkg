// Package workflow 提供工作流执行器实现。
// 生成摘要：
// 1) 提供常用节点处理器实现
// 2) 提供内存存储实现（用于开发/测试）
// 3) 提供条件评估器实现
package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MemoryDefinitionRepository 内存工作流定义存储。
type MemoryDefinitionRepository struct {
	definitions map[string]map[int]*Definition // id -> version -> definition
	latest      map[string]int                 // id -> latest version
	mu          sync.RWMutex
}

// NewMemoryDefinitionRepository 创建内存定义存储。
func NewMemoryDefinitionRepository() *MemoryDefinitionRepository {
	return &MemoryDefinitionRepository{
		definitions: make(map[string]map[int]*Definition),
		latest:      make(map[string]int),
	}
}

// Save 保存定义。
func (r *MemoryDefinitionRepository) Save(ctx context.Context, def *Definition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.definitions[def.ID]; !ok {
		r.definitions[def.ID] = make(map[int]*Definition)
	}

	currentLatest := r.latest[def.ID]
	if def.Version <= currentLatest {
		def.Version = currentLatest + 1
	}

	r.definitions[def.ID][def.Version] = def
	r.latest[def.ID] = def.Version

	return nil
}

// Get 获取定义。
func (r *MemoryDefinitionRepository) Get(ctx context.Context, id string, version int) (*Definition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.definitions[id]
	if !ok {
		return nil, fmt.Errorf("workflow definition not found: %s", id)
	}

	def, ok := versions[version]
	if !ok {
		return nil, fmt.Errorf("workflow definition version not found: %s v%d", id, version)
	}

	return def, nil
}

// GetLatest 获取最新版本定义。
func (r *MemoryDefinitionRepository) GetLatest(ctx context.Context, id string) (*Definition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	version, ok := r.latest[id]
	if !ok {
		return nil, fmt.Errorf("workflow definition not found: %s", id)
	}

	return r.definitions[id][version], nil
}

// List 列出定义。
func (r *MemoryDefinitionRepository) List(ctx context.Context, limit, offset int) ([]*Definition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Definition
	count := 0
	for id := range r.latest {
		if count >= offset && len(result) < limit {
			result = append(result, r.definitions[id][r.latest[id]])
		}
		count++
	}

	return result, nil
}

// MemoryInstanceRepository 内存工作流实例存储。
type MemoryInstanceRepository struct {
	instances     map[string]*Instance
	byBusinessKey map[string]string // businessKey -> instanceID
	mu            sync.RWMutex
}

// NewMemoryInstanceRepository 创建内存实例存储。
func NewMemoryInstanceRepository() *MemoryInstanceRepository {
	return &MemoryInstanceRepository{
		instances:     make(map[string]*Instance),
		byBusinessKey: make(map[string]string),
	}
}

// Save 保存实例。
func (r *MemoryInstanceRepository) Save(ctx context.Context, instance *Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.instances[instance.ID] = instance
	if instance.BusinessKey != "" {
		r.byBusinessKey[instance.BusinessKey] = instance.ID
	}

	return nil
}

// Get 获取实例。
func (r *MemoryInstanceRepository) Get(ctx context.Context, id string) (*Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instance, ok := r.instances[id]
	if !ok {
		return nil, fmt.Errorf("workflow instance not found: %s", id)
	}

	return instance, nil
}

// GetByBusinessKey 通过业务键获取实例。
func (r *MemoryInstanceRepository) GetByBusinessKey(ctx context.Context, businessKey string) (*Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instanceID, ok := r.byBusinessKey[businessKey]
	if !ok {
		return nil, fmt.Errorf("workflow instance not found for business key: %s", businessKey)
	}

	return r.instances[instanceID], nil
}

// List 列出实例。
func (r *MemoryInstanceRepository) List(ctx context.Context, status Status, limit, offset int) ([]*Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Instance
	count := 0
	for _, instance := range r.instances {
		if status == "" || instance.Status == status {
			if count >= offset && len(result) < limit {
				result = append(result, instance)
			}
			count++
		}
	}

	return result, nil
}

// Update 更新实例。
func (r *MemoryInstanceRepository) Update(ctx context.Context, instance *Instance) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.instances[instance.ID] = instance
	return nil
}

// SimpleConditionEvaluator 简单条件评估器。
type SimpleConditionEvaluator struct {
	logger *slog.Logger
}

// NewSimpleConditionEvaluator 创建简单条件评估器。
func NewSimpleConditionEvaluator(logger *slog.Logger) *SimpleConditionEvaluator {
	if logger == nil {
		logger = slog.Default()
	}
	return &SimpleConditionEvaluator{logger: logger}
}

// Evaluate 评估条件表达式。
// 支持的格式：
// - "field == value"
// - "field != value"
// - "field > value" (数值)
// - "field < value" (数值)
// - "field >= value" (数值)
// - "field <= value" (数值)
// - "field contains value"
// - "field startsWith value"
// - "field endsWith value"
func (e *SimpleConditionEvaluator) Evaluate(expression string, data map[string]any) (bool, error) {
	// 解析表达式
	operators := []string{">=", "<=", "!=", "==", ">", "<", " contains ", " startsWith ", " endsWith "}
	var operator string
	var parts []string

	for _, op := range operators {
		if strings.Contains(expression, op) {
			operator = strings.TrimSpace(op)
			parts = strings.SplitN(expression, op, 2)
			break
		}
	}

	if len(parts) != 2 {
		return false, fmt.Errorf("invalid expression format: %s", expression)
	}

	field := strings.TrimSpace(parts[0])
	expectedValue := strings.TrimSpace(parts[1])

	// 获取实际值
	actualValue, ok := e.getNestedValue(data, field)
	if !ok {
		return false, nil
	}

	// 比较
	switch operator {
	case "==":
		return e.equals(actualValue, expectedValue), nil
	case "!=":
		return !e.equals(actualValue, expectedValue), nil
	case ">":
		return e.compare(actualValue, expectedValue) > 0, nil
	case "<":
		return e.compare(actualValue, expectedValue) < 0, nil
	case ">=":
		return e.compare(actualValue, expectedValue) >= 0, nil
	case "<=":
		return e.compare(actualValue, expectedValue) <= 0, nil
	case "contains":
		return strings.Contains(fmt.Sprintf("%v", actualValue), expectedValue), nil
	case "startsWith":
		return strings.HasPrefix(fmt.Sprintf("%v", actualValue), expectedValue), nil
	case "endsWith":
		return strings.HasSuffix(fmt.Sprintf("%v", actualValue), expectedValue), nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", operator)
	}
}

// getNestedValue 获取嵌套字段值。
func (e *SimpleConditionEvaluator) getNestedValue(data map[string]any, field string) (any, bool) {
	parts := strings.Split(field, ".")
	current := any(data)

	for _, part := range parts {
		// 处理数组索引
		indexMatch := regexp.MustCompile(`^(\w+)\[(\d+)\]$`).FindStringSubmatch(part)
		if indexMatch != nil {
			part = indexMatch[1]
			index, _ := strconv.Atoi(indexMatch[2])

			m, ok := current.(map[string]any)
			if !ok {
				return nil, false
			}

			arr, ok := m[part].([]any)
			if !ok || index >= len(arr) {
				return nil, false
			}

			current = arr[index]
			continue
		}

		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}

		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

// equals 比较两个值是否相等。
func (e *SimpleConditionEvaluator) equals(actual any, expected string) bool {
	actualStr := fmt.Sprintf("%v", actual)

	// 去除引号
	expected = strings.Trim(expected, "\"'")

	return actualStr == expected
}

// compare 比较两个数值。
func (e *SimpleConditionEvaluator) compare(actual any, expected string) int {
	var actualFloat, expectedFloat float64

	// 转换实际值
	switch v := actual.(type) {
	case int:
		actualFloat = float64(v)
	case int64:
		actualFloat = float64(v)
	case float64:
		actualFloat = v
	case string:
		actualFloat, _ = strconv.ParseFloat(v, 64)
	default:
		return 0
	}

	// 转换期望值
	expectedFloat, _ = strconv.ParseFloat(strings.Trim(expected, "\"'"), 64)

	if actualFloat > expectedFloat {
		return 1
	} else if actualFloat < expectedFloat {
		return -1
	}
	return 0
}

// FuncHandler 函数处理器，将普通函数包装为节点处理器。
type FuncHandler struct {
	name     string
	execFunc func(ctx *ExecutionContext) (*ExecutionResult, error)
	compFunc func(ctx *ExecutionContext) error
}

// NewFuncHandler 创建函数处理器。
func NewFuncHandler(name string, execFunc func(ctx *ExecutionContext) (*ExecutionResult, error)) *FuncHandler {
	return &FuncHandler{
		name:     name,
		execFunc: execFunc,
	}
}

// WithCompensation 设置补偿函数。
func (h *FuncHandler) WithCompensation(compFunc func(ctx *ExecutionContext) error) *FuncHandler {
	h.compFunc = compFunc
	return h
}

// Execute 执行节点。
func (h *FuncHandler) Execute(ctx *ExecutionContext) (*ExecutionResult, error) {
	return h.execFunc(ctx)
}

// Compensate 补偿执行。
func (h *FuncHandler) Compensate(ctx *ExecutionContext) error {
	if h.compFunc != nil {
		return h.compFunc(ctx)
	}
	return nil
}

// HTTPHandler HTTP 调用处理器。
type HTTPHandler struct {
	logger *slog.Logger
}

// NewHTTPHandler 创建 HTTP 处理器。
func NewHTTPHandler(logger *slog.Logger) *HTTPHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPHandler{logger: logger}
}

// Execute 执行 HTTP 调用。
func (h *HTTPHandler) Execute(ctx *ExecutionContext) (*ExecutionResult, error) {
	// 从节点配置获取 HTTP 参数
	config := ctx.Node.Config
	if config == nil {
		return nil, errors.New("HTTP handler requires config")
	}

	method := strings.ToUpper(getStringConfig(config, "method"))
	urlStr := getStringConfig(config, "url")

	if method == "" || urlStr == "" {
		return nil, errors.New("HTTP handler requires method and url in config")
	}

	logger := h.logger
	if ctx.Logger != nil {
		logger = ctx.Logger
	}
	if logger == nil {
		logger = slog.Default()
	}

	urlStr = replaceVariables(urlStr, ctx.Input)
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	// 处理 query 参数
	if query := getMapConfig(config, "query"); len(query) > 0 {
		values := parsedURL.Query()
		for k, v := range query {
			values.Set(k, replaceVariables(fmt.Sprintf("%v", v), ctx.Input))
		}
		parsedURL.RawQuery = values.Encode()
	}

	// 处理 headers
	headers := http.Header{}
	if headerConfig := getMapConfig(config, "headers"); len(headerConfig) > 0 {
		for k, v := range headerConfig {
			headers.Set(k, replaceVariables(fmt.Sprintf("%v", v), ctx.Input))
		}
	}

	contentType := getStringConfig(config, "content_type")
	bodyReader, err := buildRequestBody(config["body"], ctx.Input, &contentType)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx.Context, method, parsedURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if contentType != "" && headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", contentType)
	}
	req.Header = headers

	timeout := 15 * time.Second
	if d, ok := parseDuration(config["timeout"]); ok {
		timeout = d
	}
	client := &http.Client{Timeout: timeout}

	logger.InfoContext(ctx.Context, "HTTP request",
		"method", method,
		"url", parsedURL.String(),
		"timeout", timeout.String(),
		"node_id", ctx.Node.ID)

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	maxResponseBytes := int64(1024 * 1024)
	if limit, ok := parseInt64(config["max_response_bytes"]); ok && limit > 0 {
		maxResponseBytes = limit
	}

	bodyBytes, err := readResponseBody(resp.Body, maxResponseBytes)
	if err != nil {
		return nil, err
	}

	output := map[string]any{
		"status_code": resp.StatusCode,
		"duration_ms": time.Since(start).Milliseconds(),
		"body":        string(bodyBytes),
		"headers":     cloneHeaders(resp.Header),
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		var bodyJSON any
		if err := json.Unmarshal(bodyBytes, &bodyJSON); err == nil {
			output["body_json"] = bodyJSON
		}
	}

	result := &ExecutionResult{Output: output}
	if !getBoolConfig(config, "allow_non_2xx") && resp.StatusCode >= http.StatusBadRequest {
		return result, fmt.Errorf("HTTP request failed with status %d", resp.StatusCode)
	}

	return result, nil
}

// Compensate 补偿执行。
func (h *HTTPHandler) Compensate(ctx *ExecutionContext) error {
	// HTTP 调用通常不需要补偿
	return nil
}

func replaceVariables(template string, input map[string]any) string {
	if template == "" || len(input) == 0 {
		return template
	}
	result := template
	for k, v := range input {
		result = strings.ReplaceAll(result, fmt.Sprintf("${%s}", k), fmt.Sprintf("%v", v))
	}
	return result
}

func replaceVariablesInValue(value any, input map[string]any) any {
	switch v := value.(type) {
	case string:
		return replaceVariables(v, input)
	case map[string]any:
		clone := make(map[string]any, len(v))
		for k, val := range v {
			clone[k] = replaceVariablesInValue(val, input)
		}
		return clone
	case map[string]string:
		clone := make(map[string]any, len(v))
		for k, val := range v {
			clone[k] = replaceVariables(val, input)
		}
		return clone
	case []any:
		clone := make([]any, 0, len(v))
		for _, val := range v {
			clone = append(clone, replaceVariablesInValue(val, input))
		}
		return clone
	default:
		return value
	}
}

func buildRequestBody(raw any, input map[string]any, contentType *string) (io.Reader, error) {
	if raw == nil {
		return nil, nil
	}

	resolved := replaceVariablesInValue(raw, input)
	switch v := resolved.(type) {
	case string:
		if contentType != nil && *contentType == "" {
			*contentType = "text/plain; charset=utf-8"
		}
		return strings.NewReader(v), nil
	case []byte:
		if contentType != nil && *contentType == "" {
			*contentType = "application/octet-stream"
		}
		return bytes.NewReader(v), nil
	default:
		bodyBytes, err := json.Marshal(resolved)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		if contentType != nil && *contentType == "" {
			*contentType = "application/json"
		}
		return bytes.NewReader(bodyBytes), nil
	}
}

func getStringConfig(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	val, ok := config[key]
	if !ok || val == nil {
		return ""
	}
	if str, ok := val.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", val)
}

func getMapConfig(config map[string]any, key string) map[string]any {
	if config == nil {
		return nil
	}
	val, ok := config[key]
	if !ok || val == nil {
		return nil
	}

	switch typed := val.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		converted := make(map[string]any, len(typed))
		for k, v := range typed {
			converted[k] = v
		}
		return converted
	default:
		return nil
	}
}

func getBoolConfig(config map[string]any, key string) bool {
	if config == nil {
		return false
	}
	val, ok := config[key]
	if !ok || val == nil {
		return false
	}
	switch typed := val.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(typed)
		if err == nil {
			return parsed
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	}
	return false
}

func parseDuration(value any) (time.Duration, bool) {
	switch v := value.(type) {
	case time.Duration:
		return v, true
	case string:
		parsed, err := time.ParseDuration(v)
		if err == nil {
			return parsed, true
		}
	case int:
		return time.Duration(v) * time.Millisecond, true
	case int64:
		return time.Duration(v) * time.Millisecond, true
	case float64:
		return time.Duration(v) * time.Millisecond, true
	case json.Number:
		parsed, err := v.Int64()
		if err == nil {
			return time.Duration(parsed) * time.Millisecond, true
		}
	}
	return 0, false
}

func parseInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return parsed, true
		}
	case json.Number:
		parsed, err := v.Int64()
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func readResponseBody(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = 1024 * 1024
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response body exceeds limit of %d bytes", limit)
	}
	return body, nil
}

func cloneHeaders(headers http.Header) map[string][]string {
	clone := make(map[string][]string, len(headers))
	for k, v := range headers {
		copied := make([]string, len(v))
		copy(copied, v)
		clone[k] = copied
	}
	return clone
}

// ScriptHandler 脚本执行处理器。
type ScriptHandler struct {
	scripts map[string]func(ctx *ExecutionContext) (*ExecutionResult, error)
	mu      sync.RWMutex
	logger  *slog.Logger
}

// NewScriptHandler 创建脚本处理器。
func NewScriptHandler(logger *slog.Logger) *ScriptHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ScriptHandler{
		scripts: make(map[string]func(ctx *ExecutionContext) (*ExecutionResult, error)),
		logger:  logger,
	}
}

// RegisterScript 注册脚本。
func (h *ScriptHandler) RegisterScript(name string, fn func(ctx *ExecutionContext) (*ExecutionResult, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.scripts[name] = fn
}

// Execute 执行脚本。
func (h *ScriptHandler) Execute(ctx *ExecutionContext) (*ExecutionResult, error) {
	if ctx.Node.Config == nil {
		return nil, errors.New("script handler requires config")
	}
	scriptName, _ := ctx.Node.Config["script"].(string)
	if scriptName == "" {
		return nil, errors.New("script name not specified")
	}

	h.mu.RLock()
	script, ok := h.scripts[scriptName]
	h.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("script not found: %s", scriptName)
	}

	return script(ctx)
}

// Compensate 补偿执行。
func (h *ScriptHandler) Compensate(ctx *ExecutionContext) error {
	return nil
}

// NotificationHandler 通知处理器。
type NotificationHandler struct {
	sendFunc func(ctx context.Context, channel, recipient, subject, content string) error
	logger   *slog.Logger
}

// NewNotificationHandler 创建通知处理器。
func NewNotificationHandler(
	sendFunc func(ctx context.Context, channel, recipient, subject, content string) error,
	logger *slog.Logger,
) *NotificationHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &NotificationHandler{
		sendFunc: sendFunc,
		logger:   logger,
	}
}

// Execute 执行通知发送。
func (h *NotificationHandler) Execute(ctx *ExecutionContext) (*ExecutionResult, error) {
	if h.sendFunc == nil {
		return nil, errors.New("notification handler requires sendFunc")
	}
	config := ctx.Node.Config
	if config == nil {
		return nil, errors.New("notification handler requires config")
	}

	channel, _ := config["channel"].(string)
	recipient, _ := config["recipient"].(string)
	subject, _ := config["subject"].(string)
	content, _ := config["content"].(string)

	// 从输入替换变量
	for k, v := range ctx.Input {
		recipient = strings.ReplaceAll(recipient, fmt.Sprintf("${%s}", k), fmt.Sprintf("%v", v))
		subject = strings.ReplaceAll(subject, fmt.Sprintf("${%s}", k), fmt.Sprintf("%v", v))
		content = strings.ReplaceAll(content, fmt.Sprintf("${%s}", k), fmt.Sprintf("%v", v))
	}

	if err := h.sendFunc(ctx.Context, channel, recipient, subject, content); err != nil {
		return nil, fmt.Errorf("failed to send notification: %w", err)
	}

	h.logger.InfoContext(ctx.Context, "notification sent",
		"channel", channel,
		"recipient", recipient,
		"node_id", ctx.Node.ID)

	return &ExecutionResult{
		Output: map[string]any{
			"sent":      true,
			"channel":   channel,
			"recipient": recipient,
		},
	}, nil
}

// Compensate 补偿执行。
func (h *NotificationHandler) Compensate(ctx *ExecutionContext) error {
	// 通知发送不需要补偿
	return nil
}

// DelayHandler 延时处理器。
type DelayHandler struct {
	logger *slog.Logger
}

// NewDelayHandler 创建延时处理器。
func NewDelayHandler(logger *slog.Logger) *DelayHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DelayHandler{logger: logger}
}

// Execute 执行延时。
func (h *DelayHandler) Execute(ctx *ExecutionContext) (*ExecutionResult, error) {
	if ctx.Node.Config == nil {
		return nil, errors.New("delay handler requires config")
	}
	durationStr, _ := ctx.Node.Config["duration"].(string)
	if durationStr == "" {
		return nil, errors.New("delay handler requires duration in config")
	}

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return nil, fmt.Errorf("invalid duration: %w", err)
	}

	h.logger.InfoContext(ctx.Context, "delaying workflow",
		"duration", duration,
		"node_id", ctx.Node.ID)

	select {
	case <-ctx.Context.Done():
		return nil, ctx.Context.Err()
	case <-time.After(duration):
		return &ExecutionResult{
			Output: map[string]any{
				"delayed":  true,
				"duration": duration.String(),
			},
		}, nil
	}
}

// Compensate 补偿执行。
func (h *DelayHandler) Compensate(ctx *ExecutionContext) error {
	return nil
}

// TransformHandler 数据转换处理器。
type TransformHandler struct {
	logger *slog.Logger
}

// NewTransformHandler 创建数据转换处理器。
func NewTransformHandler(logger *slog.Logger) *TransformHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &TransformHandler{logger: logger}
}

// Execute 执行数据转换。
func (h *TransformHandler) Execute(ctx *ExecutionContext) (*ExecutionResult, error) {
	if ctx.Node.Config == nil {
		return nil, errors.New("transform handler requires config")
	}
	mappings, ok := ctx.Node.Config["mappings"].(map[string]any)
	if !ok {
		return nil, errors.New("transform handler requires mappings in config")
	}

	output := make(map[string]any)
	for targetField, sourceExpr := range mappings {
		sourceField, ok := sourceExpr.(string)
		if !ok {
			continue
		}

		// 支持简单的字段映射和表达式
		if strings.HasPrefix(sourceField, "${") && strings.HasSuffix(sourceField, "}") {
			fieldName := sourceField[2 : len(sourceField)-1]
			if value, exists := ctx.Input[fieldName]; exists {
				output[targetField] = value
			}
		} else {
			output[targetField] = sourceField
		}
	}

	return &ExecutionResult{Output: output}, nil
}

// Compensate 补偿执行。
func (h *TransformHandler) Compensate(ctx *ExecutionContext) error {
	return nil
}

// ValidationHandler 验证处理器。
type ValidationHandler struct {
	logger *slog.Logger
}

// NewValidationHandler 创建验证处理器。
func NewValidationHandler(logger *slog.Logger) *ValidationHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ValidationHandler{logger: logger}
}

// Execute 执行验证。
func (h *ValidationHandler) Execute(ctx *ExecutionContext) (*ExecutionResult, error) {
	if ctx.Node.Config == nil {
		return nil, errors.New("validation handler requires config")
	}
	rules, ok := ctx.Node.Config["rules"].([]any)
	if !ok {
		return nil, errors.New("validation handler requires rules in config")
	}

	var validationErrors []string

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]any)
		if !ok {
			continue
		}

		field, _ := ruleMap["field"].(string)
		ruleType, _ := ruleMap["type"].(string)

		value, exists := ctx.Input[field]

		switch ruleType {
		case "required":
			if !exists || value == nil || value == "" {
				validationErrors = append(validationErrors, fmt.Sprintf("field %s is required", field))
			}
		case "min":
			minValue, _ := ruleMap["value"].(float64)
			if numValue, ok := value.(float64); ok && numValue < minValue {
				validationErrors = append(validationErrors, fmt.Sprintf("field %s must be >= %v", field, minValue))
			}
		case "max":
			maxValue, _ := ruleMap["value"].(float64)
			if numValue, ok := value.(float64); ok && numValue > maxValue {
				validationErrors = append(validationErrors, fmt.Sprintf("field %s must be <= %v", field, maxValue))
			}
		case "pattern":
			pattern, _ := ruleMap["value"].(string)
			if strValue, ok := value.(string); ok {
				if matched, _ := regexp.MatchString(pattern, strValue); !matched {
					validationErrors = append(validationErrors, fmt.Sprintf("field %s does not match pattern", field))
				}
			}
		case "type":
			expectedType, _ := ruleMap["value"].(string)
			actualType := reflect.TypeOf(value).String()
			if actualType != expectedType {
				validationErrors = append(validationErrors, fmt.Sprintf("field %s expected type %s, got %s", field, expectedType, actualType))
			}
		}
	}

	if len(validationErrors) > 0 {
		return nil, fmt.Errorf("validation failed: %s", strings.Join(validationErrors, "; "))
	}

	return &ExecutionResult{
		Output: map[string]any{
			"validated": true,
		},
	}, nil
}

// Compensate 补偿执行。
func (h *ValidationHandler) Compensate(ctx *ExecutionContext) error {
	return nil
}
