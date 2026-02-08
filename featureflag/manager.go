package featureflag

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
)

const (
	ScopeModeAll = "all"
	ScopeModeAny = "any"

	reasonMissing  = "missing"
	reasonDisabled = "disabled"
	reasonDenied   = "deny"
	reasonAllowed  = "allow"
	reasonTime     = "time"
	reasonScope    = "scope"
	reasonRollout  = "rollout"
	reasonEnabled  = "enabled"
	reasonDefault  = "default"
)

// Flag 定义功能开关的配置。
type Flag struct {
	Name           string    // 开关名称（唯一）。
	Enabled        bool      // 是否启用。
	Rollout        int       // 灰度百分比 (0-100)。
	StartAt        time.Time // 生效开始时间。
	EndAt          time.Time // 生效结束时间。
	AllowUsers     []string  // 强制开启的用户列表。
	DenyUsers      []string  // 强制关闭的用户列表。
	AllowTenants   []string  // 强制开启的租户列表。
	DenyTenants    []string  // 强制关闭的租户列表。
	AllowRoles     []string  // 强制开启的角色列表。
	DenyRoles      []string  // 强制关闭的角色列表。
	AllowIPs       []string  // 强制开启的 IP 列表。
	DenyIPs        []string  // 强制关闭的 IP 列表。
	RequiredScopes []string  // 需要满足的权限范围。
	ScopeMode      string    // 权限匹配模式 (all/any)。
	Description    string    // 开关描述。
}

// Decision 表示一次开关判断的结果。
type Decision struct {
	Enabled bool
	Reason  string
}

// EvalContext 定义评估上下文。
type EvalContext struct {
	UserID   string
	TenantID string
	Role     string
	IP       string
	Scopes   string
	Seed     string
}

// EvalOption 设置评估上下文参数。
type EvalOption func(*EvalContext)

// WithUserID 设置评估用户 ID。
func WithUserID(userID string) EvalOption {
	return func(c *EvalContext) {
		c.UserID = userID
	}
}

// WithTenantID 设置评估租户 ID。
func WithTenantID(tenantID string) EvalOption {
	return func(c *EvalContext) {
		c.TenantID = tenantID
	}
}

// WithRole 设置评估角色。
func WithRole(role string) EvalOption {
	return func(c *EvalContext) {
		c.Role = role
	}
}

// WithIP 设置评估 IP。
func WithIP(ip string) EvalOption {
	return func(c *EvalContext) {
		c.IP = ip
	}
}

// WithScopes 设置评估权限范围。
func WithScopes(scopes string) EvalOption {
	return func(c *EvalContext) {
		c.Scopes = scopes
	}
}

// WithSeed 设置评估随机种子。
func WithSeed(seed string) EvalOption {
	return func(c *EvalContext) {
		c.Seed = seed
	}
}

// Option 定义 Manager 构造参数。
type Option func(*Manager)

// WithLogger 注入日志记录器。
func WithLogger(logger *logging.Logger) Option {
	return func(m *Manager) {
		if logger != nil {
			m.logger = logger
		}
	}
}

// WithMetrics 注入指标采集器。
func WithMetrics(metricsInstance *metrics.Metrics) Option {
	return func(m *Manager) {
		if metricsInstance == nil {
			return
		}
		m.metrics = &flagMetrics{
			evaluations: metricsInstance.NewCounterVec(&prometheus.CounterOpts{
				Namespace: "pkg",
				Subsystem: "feature_flag",
				Name:      "evaluations_total",
				Help:      "Total number of feature flag evaluations",
			}, []string{"flag", "enabled", "reason"}),
		}
	}
}

// WithDefaultEnabled 设置默认开关行为。
func WithDefaultEnabled(enabled bool) Option {
	return func(m *Manager) {
		m.defaultEnabled = enabled
	}
}

// WithEnabled 设置全局是否启用开关系统。
func WithEnabled(enabled bool) Option {
	return func(m *Manager) {
		m.enabled = enabled
	}
}

type flagMetrics struct {
	evaluations *prometheus.CounterVec
}

type compiledFlag struct {
	flag           Flag
	allowUsers     map[string]struct{}
	denyUsers      map[string]struct{}
	allowTenants   map[string]struct{}
	denyTenants    map[string]struct{}
	allowRoles     map[string]struct{}
	denyRoles      map[string]struct{}
	allowIPs       map[string]struct{}
	denyIPs        map[string]struct{}
	requiredScopes map[string]struct{}
	scopeMode      string
	rollout        int
}

// Manager 提供功能开关的统一管理能力。
type Manager struct {
	mu             sync.RWMutex
	flags          map[string]compiledFlag
	logger         *logging.Logger
	metrics        *flagMetrics
	defaultEnabled bool
	enabled        bool
}

var defaultManager *Manager
var defaultOnce sync.Once

// Default 返回默认 Manager 实例。
func Default() *Manager {
	defaultOnce.Do(func() {
		defaultManager = NewManager()
	})
	return defaultManager
}

// SetDefault 设置默认 Manager。
func SetDefault(m *Manager) {
	if m == nil {
		return
	}
	defaultManager = m
}

// NewManager 创建功能开关管理器。
func NewManager(opts ...Option) *Manager {
	manager := &Manager{
		flags:          make(map[string]compiledFlag),
		logger:         logging.Default(),
		defaultEnabled: false,
		enabled:        true,
	}
	for _, opt := range opts {
		opt(manager)
	}

	return manager
}

// SetEnabled 设置开关系统是否启用。
func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
}

// Load 批量加载开关配置（覆盖当前配置）。
func (m *Manager) Load(flags []Flag) {
	compiled := make(map[string]compiledFlag, len(flags))
	for _, flag := range flags {
		if flag.Name == "" {
			continue
		}
		compiled[flag.Name] = compileFlag(flag)
	}

	m.mu.Lock()
	m.flags = compiled
	m.mu.Unlock()
}

// SetFlag 设置或更新单个开关。
func (m *Manager) SetFlag(flag Flag) {
	if flag.Name == "" {
		return
	}
	m.mu.Lock()
	m.flags[flag.Name] = compileFlag(flag)
	m.mu.Unlock()
}

// RemoveFlag 删除开关。
func (m *Manager) RemoveFlag(name string) {
	m.mu.Lock()
	delete(m.flags, name)
	m.mu.Unlock()
}

// IsEnabled 判断指定开关是否启用。
func (m *Manager) IsEnabled(ctx context.Context, name string, opts ...EvalOption) bool {
	return m.Evaluate(ctx, name, opts...).Enabled
}

// Evaluate 返回开关判断结果与原因。
func (m *Manager) Evaluate(ctx context.Context, name string, opts ...EvalOption) Decision {
	evalCtx := buildEvalContext(ctx, opts...)

	m.mu.RLock()
	enabled := m.enabled
	defaultEnabled := m.defaultEnabled
	flag, ok := m.flags[name]
	m.mu.RUnlock()

	if !enabled {
		return m.recordDecision(name, Decision{Enabled: defaultEnabled, Reason: reasonDisabled})
	}
	if !ok {
		return m.recordDecision(name, Decision{Enabled: defaultEnabled, Reason: reasonMissing})
	}

	now := time.Now()
	if matchAny(flag.denyUsers, evalCtx.UserID) || matchAny(flag.denyTenants, evalCtx.TenantID) || matchAny(flag.denyRoles, evalCtx.Role) || matchAny(flag.denyIPs, evalCtx.IP) {
		return m.recordDecision(name, Decision{Enabled: false, Reason: reasonDenied})
	}

	if matchAny(flag.allowUsers, evalCtx.UserID) || matchAny(flag.allowTenants, evalCtx.TenantID) || matchAny(flag.allowRoles, evalCtx.Role) || matchAny(flag.allowIPs, evalCtx.IP) {
		return m.recordDecision(name, Decision{Enabled: true, Reason: reasonAllowed})
	}

	if !flag.flag.Enabled {
		return m.recordDecision(name, Decision{Enabled: false, Reason: reasonDisabled})
	}

	if !flag.flag.StartAt.IsZero() && now.Before(flag.flag.StartAt) {
		return m.recordDecision(name, Decision{Enabled: false, Reason: reasonTime})
	}
	if !flag.flag.EndAt.IsZero() && now.After(flag.flag.EndAt) {
		return m.recordDecision(name, Decision{Enabled: false, Reason: reasonTime})
	}

	if len(flag.requiredScopes) > 0 && !matchScopes(flag.requiredScopes, evalCtx.Scopes, flag.scopeMode) {
		return m.recordDecision(name, Decision{Enabled: false, Reason: reasonScope})
	}

	if flag.rollout < 100 {
		seed := resolveSeed(evalCtx)
		if !matchRollout(name, seed, flag.rollout) {
			return m.recordDecision(name, Decision{Enabled: false, Reason: reasonRollout})
		}
	}

	return m.recordDecision(name, Decision{Enabled: true, Reason: reasonEnabled})
}

func (m *Manager) recordDecision(name string, decision Decision) Decision {
	if m.metrics != nil && m.metrics.evaluations != nil {
		m.metrics.evaluations.WithLabelValues(name, strconv.FormatBool(decision.Enabled), decision.Reason).Inc()
	}
	return decision
}

func buildEvalContext(ctx context.Context, opts ...EvalOption) EvalContext {
	evalCtx := EvalContext{
		UserID:   contextx.GetUserID(ctx),
		TenantID: contextx.GetTenantID(ctx),
		Role:     contextx.GetRole(ctx),
		IP:       contextx.GetIP(ctx),
		Scopes:   contextx.GetScopes(ctx),
		Seed:     "",
	}
	for _, opt := range opts {
		opt(&evalCtx)
	}
	return evalCtx
}

func compileFlag(flag Flag) compiledFlag {
	return compiledFlag{
		flag:           flag,
		allowUsers:     normalizeSet(flag.AllowUsers),
		denyUsers:      normalizeSet(flag.DenyUsers),
		allowTenants:   normalizeSet(flag.AllowTenants),
		denyTenants:    normalizeSet(flag.DenyTenants),
		allowRoles:     normalizeSet(flag.AllowRoles),
		denyRoles:      normalizeSet(flag.DenyRoles),
		allowIPs:       normalizeSet(flag.AllowIPs),
		denyIPs:        normalizeSet(flag.DenyIPs),
		requiredScopes: normalizeSet(flag.RequiredScopes),
		scopeMode:      normalizeScopeMode(flag.ScopeMode),
		rollout:        normalizeRollout(flag.Enabled, flag.Rollout),
	}
}

func normalizeSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" {
			continue
		}
		result[key] = struct{}{}
	}
	return result
}

func normalizeScopeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ScopeModeAny:
		return ScopeModeAny
	default:
		return ScopeModeAll
	}
}

func normalizeRollout(enabled bool, rollout int) int {
	if rollout <= 0 {
		if enabled {
			return 100
		}
		return 0
	}
	if rollout > 100 {
		return 100
	}
	return rollout
}

func matchAny(set map[string]struct{}, value string) bool {
	if len(set) == 0 || value == "" {
		return false
	}
	_, ok := set[value]
	return ok
}

func matchScopes(required map[string]struct{}, scopes string, mode string) bool {
	if len(required) == 0 {
		return true
	}

	scopeSet := normalizeSet(strings.Split(scopes, ","))
	if len(scopeSet) == 0 {
		return false
	}

	if mode == ScopeModeAny {
		for scope := range required {
			if _, ok := scopeSet[scope]; ok {
				return true
			}
		}
		return false
	}

	for scope := range required {
		if _, ok := scopeSet[scope]; !ok {
			return false
		}
	}
	return true
}

func resolveSeed(evalCtx EvalContext) string {
	if evalCtx.Seed != "" {
		return evalCtx.Seed
	}
	if evalCtx.UserID != "" {
		return evalCtx.UserID
	}
	if evalCtx.TenantID != "" {
		return evalCtx.TenantID
	}
	if evalCtx.IP != "" {
		return evalCtx.IP
	}
	return "global"
}

func matchRollout(flagName, seed string, rollout int) bool {
	if rollout >= 100 {
		return true
	}
	if rollout <= 0 {
		return false
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(fmt.Sprintf("%s:%s", flagName, seed)))
	value := int(h.Sum32() % 100)
	return value < rollout
}
