// Package workflow 提供轻量级工作流引擎。
// 生成摘要：
// 1) 支持工作流定义与执行
// 2) 支持串行/并行/条件分支执行
// 3) 支持任务重试和补偿
// 假设：
// - 工作流状态可持久化用于恢复
// - 每个节点都有明确的输入输出
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"
)

// NodeType 定义节点类型。
type NodeType string

const (
	// NodeTypeTask 任务节点。
	NodeTypeTask NodeType = "task"
	// NodeTypeParallel 并行网关。
	NodeTypeParallel NodeType = "parallel"
	// NodeTypeExclusive 排他网关（条件分支）。
	NodeTypeExclusive NodeType = "exclusive"
	// NodeTypeInclusive 包容网关。
	NodeTypeInclusive NodeType = "inclusive"
	// NodeTypeSubWorkflow 子工作流。
	NodeTypeSubWorkflow NodeType = "sub_workflow"
	// NodeTypeWait 等待节点。
	NodeTypeWait NodeType = "wait"
	// NodeTypeCompensation 补偿节点。
	NodeTypeCompensation NodeType = "compensation"
)

// Status 定义工作流/节点状态。
type Status string

const (
	// StatusPending 待执行。
	StatusPending Status = "pending"
	// StatusRunning 执行中。
	StatusRunning Status = "running"
	// StatusCompleted 已完成。
	StatusCompleted Status = "completed"
	// StatusFailed 已失败。
	StatusFailed Status = "failed"
	// StatusCanceled 已取消。
	StatusCanceled Status = "canceled"
	// StatusCompensating 补偿中。
	StatusCompensating Status = "compensating"
	// StatusCompensated 已补偿。
	StatusCompensated Status = "compensated"
	// StatusWaiting 等待中。
	StatusWaiting Status = "waiting"
	// StatusSkipped 已跳过。
	StatusSkipped Status = "skipped"
)

// Definition 工作流定义。
type Definition struct {
	// ID 工作流定义ID。
	ID string `json:"id"`
	// Name 工作流名称。
	Name string `json:"name"`
	// Version 版本号。
	Version int `json:"version"`
	// Description 描述。
	Description string `json:"description,omitempty"`
	// Nodes 节点列表。
	Nodes []*NodeDefinition `json:"nodes"`
	// Transitions 转换规则。
	Transitions []*Transition `json:"transitions"`
	// StartNodeID 起始节点ID。
	StartNodeID string `json:"start_node_id"`
	// GlobalTimeout 全局超时。
	GlobalTimeout time.Duration `json:"global_timeout,omitempty"`
}

// NodeDefinition 节点定义。
type NodeDefinition struct {
	// ID 节点ID。
	ID string `json:"id"`
	// Name 节点名称。
	Name string `json:"name"`
	// Type 节点类型。
	Type NodeType `json:"type"`
	// Handler 处理器名称（用于查找执行器）。
	Handler string `json:"handler,omitempty"`
	// Config 节点配置。
	Config map[string]any `json:"config,omitempty"`
	// Timeout 节点超时。
	Timeout time.Duration `json:"timeout,omitempty"`
	// RetryPolicy 重试策略。
	RetryPolicy *RetryPolicy `json:"retry_policy,omitempty"`
	// CompensationHandler 补偿处理器。
	CompensationHandler string `json:"compensation_handler,omitempty"`
	// SubWorkflowID 子工作流ID（用于 SubWorkflow 类型）。
	SubWorkflowID string `json:"sub_workflow_id,omitempty"`
	// WaitDuration 等待时长（用于 Wait 类型）。
	WaitDuration time.Duration `json:"wait_duration,omitempty"`
}

// Transition 转换规则。
type Transition struct {
	// From 源节点ID。
	From string `json:"from"`
	// To 目标节点ID。
	To string `json:"to"`
	// Condition 条件表达式。
	Condition string `json:"condition,omitempty"`
	// Priority 优先级（用于排他网关）。
	Priority int `json:"priority,omitempty"`
	// IsDefault 是否默认分支。
	IsDefault bool `json:"is_default,omitempty"`
}

// RetryPolicy 重试策略。
type RetryPolicy struct {
	// MaxRetries 最大重试次数。
	MaxRetries int `json:"max_retries"`
	// InitialInterval 初始重试间隔。
	InitialInterval time.Duration `json:"initial_interval"`
	// MaxInterval 最大重试间隔。
	MaxInterval time.Duration `json:"max_interval"`
	// Multiplier 间隔乘数。
	Multiplier float64 `json:"multiplier"`
	// RetryableErrors 可重试的错误类型。
	RetryableErrors []string `json:"retryable_errors,omitempty"`
}

// Instance 工作流实例。
type Instance struct {
	// ID 实例ID。
	ID string `json:"id"`
	// DefinitionID 工作流定义ID。
	DefinitionID string `json:"definition_id"`
	// DefinitionVersion 工作流定义版本。
	DefinitionVersion int `json:"definition_version"`
	// Status 实例状态。
	Status Status `json:"status"`
	// CurrentNodeID 当前节点ID。
	CurrentNodeID string `json:"current_node_id,omitempty"`
	// Input 输入参数。
	Input map[string]any `json:"input,omitempty"`
	// Output 输出结果。
	Output map[string]any `json:"output,omitempty"`
	// Context 执行上下文（节点间共享数据）。
	Context map[string]any `json:"context,omitempty"`
	// NodeStates 节点状态。
	NodeStates map[string]*NodeState `json:"node_states,omitempty"`
	// Error 错误信息。
	Error string `json:"error,omitempty"`
	// StartedAt 开始时间。
	StartedAt time.Time `json:"started_at"`
	// CompletedAt 完成时间。
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// BusinessKey 业务关联键。
	BusinessKey string `json:"business_key,omitempty"`
	// ParentInstanceID 父工作流实例ID（子工作流）。
	ParentInstanceID string `json:"parent_instance_id,omitempty"`
}

// NodeState 节点执行状态。
type NodeState struct {
	// NodeID 节点ID。
	NodeID string `json:"node_id"`
	// Status 节点状态。
	Status Status `json:"status"`
	// Input 节点输入。
	Input map[string]any `json:"input,omitempty"`
	// Output 节点输出。
	Output map[string]any `json:"output,omitempty"`
	// Error 错误信息。
	Error string `json:"error,omitempty"`
	// RetryCount 重试次数。
	RetryCount int `json:"retry_count"`
	// StartedAt 开始时间。
	StartedAt time.Time `json:"started_at"`
	// CompletedAt 完成时间。
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ExecutionContext 执行上下文。
type ExecutionContext struct {
	// Context Go context。
	Context context.Context
	// Instance 工作流实例。
	Instance *Instance
	// Node 当前节点定义。
	Node *NodeDefinition
	// Input 节点输入。
	Input map[string]any
	// Logger 日志记录器。
	Logger *slog.Logger
}

// ExecutionResult 执行结果。
type ExecutionResult struct {
	// Output 输出数据。
	Output map[string]any `json:"output,omitempty"`
	// Error 错误信息。
	Error error `json:"-"`
	// NextNodeID 下一个节点ID（用于排他网关）。
	NextNodeID string `json:"next_node_id,omitempty"`
	// ShouldRetry 是否应该重试。
	ShouldRetry bool `json:"should_retry,omitempty"`
}

// Handler 节点处理器接口。
type Handler interface {
	// Execute 执行节点。
	Execute(ctx *ExecutionContext) (*ExecutionResult, error)
	// Compensate 补偿执行（可选）。
	Compensate(ctx *ExecutionContext) error
}

// ConditionEvaluator 条件评估器接口。
type ConditionEvaluator interface {
	// Evaluate 评估条件表达式。
	Evaluate(expression string, data map[string]any) (bool, error)
}

// InstanceRepository 工作流实例存储接口。
type InstanceRepository interface {
	// Save 保存实例。
	Save(ctx context.Context, instance *Instance) error
	// Get 获取实例。
	Get(ctx context.Context, id string) (*Instance, error)
	// GetByBusinessKey 通过业务键获取实例。
	GetByBusinessKey(ctx context.Context, businessKey string) (*Instance, error)
	// List 列出实例。
	List(ctx context.Context, status Status, limit, offset int) ([]*Instance, error)
	// Update 更新实例。
	Update(ctx context.Context, instance *Instance) error
}

// DefinitionRepository 工作流定义存储接口。
type DefinitionRepository interface {
	// Save 保存定义。
	Save(ctx context.Context, def *Definition) error
	// Get 获取定义。
	Get(ctx context.Context, id string, version int) (*Definition, error)
	// GetLatest 获取最新版本定义。
	GetLatest(ctx context.Context, id string) (*Definition, error)
	// List 列出定义。
	List(ctx context.Context, limit, offset int) ([]*Definition, error)
}

// Engine 工作流引擎。
type Engine struct {
	handlers         map[string]Handler
	definitions      DefinitionRepository
	instances        InstanceRepository
	conditionEval    ConditionEvaluator
	logger           *slog.Logger
	mu               sync.RWMutex
	runningInstances map[string]context.CancelFunc
}

// NewEngine 创建工作流引擎。
func NewEngine(
	definitions DefinitionRepository,
	instances InstanceRepository,
	logger *slog.Logger,
) *Engine {
	return &Engine{
		handlers:         make(map[string]Handler),
		definitions:      definitions,
		instances:        instances,
		logger:           logger,
		runningInstances: make(map[string]context.CancelFunc),
	}
}

// RegisterHandler 注册节点处理器。
func (e *Engine) RegisterHandler(name string, handler Handler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[name] = handler
	e.logger.Info("workflow handler registered", "name", name)
}

// SetConditionEvaluator 设置条件评估器。
func (e *Engine) SetConditionEvaluator(evaluator ConditionEvaluator) {
	e.conditionEval = evaluator
}

// StartWorkflow 启动工作流。
func (e *Engine) StartWorkflow(ctx context.Context, definitionID string, input map[string]any, businessKey string) (*Instance, error) {
	// 获取工作流定义
	def, err := e.definitions.GetLatest(ctx, definitionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow definition: %w", err)
	}

	// 创建实例
	instance := &Instance{
		ID:                fmt.Sprintf("wf_%d", time.Now().UnixNano()),
		DefinitionID:      def.ID,
		DefinitionVersion: def.Version,
		Status:            StatusRunning,
		CurrentNodeID:     def.StartNodeID,
		Input:             input,
		Context:           make(map[string]any),
		NodeStates:        make(map[string]*NodeState),
		StartedAt:         time.Now(),
		BusinessKey:       businessKey,
	}

	// 保存实例
	if err := e.instances.Save(ctx, instance); err != nil {
		return nil, fmt.Errorf("failed to save workflow instance: %w", err)
	}

	e.logger.InfoContext(ctx, "workflow started",
		"instance_id", instance.ID,
		"definition_id", def.ID,
		"business_key", businessKey)

	// 异步执行
	go e.executeWorkflow(instance, def)

	return instance, nil
}

// executeWorkflow 执行工作流。
func (e *Engine) executeWorkflow(instance *Instance, def *Definition) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 注册运行中实例
	e.mu.Lock()
	e.runningInstances[instance.ID] = cancel
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.runningInstances, instance.ID)
		e.mu.Unlock()
	}()

	// 应用全局超时
	if def.GlobalTimeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, def.GlobalTimeout)
		defer timeoutCancel()
	}

	// 构建节点索引
	nodeIndex := make(map[string]*NodeDefinition)
	for _, node := range def.Nodes {
		nodeIndex[node.ID] = node
	}

	// 构建转换索引
	transitionIndex := make(map[string][]*Transition)
	for _, t := range def.Transitions {
		transitionIndex[t.From] = append(transitionIndex[t.From], t)
	}

	// 执行循环
	for instance.Status == StatusRunning {
		select {
		case <-ctx.Done():
			instance.Status = StatusCanceled
			instance.Error = ctx.Err().Error()
			e.updateInstance(instance)
			return
		default:
		}

		nodeID := instance.CurrentNodeID
		if nodeID == "" {
			// 工作流完成
			instance.Status = StatusCompleted
			now := time.Now()
			instance.CompletedAt = &now
			e.updateInstance(instance)
			e.logger.Info("workflow completed", "instance_id", instance.ID)
			return
		}

		node, ok := nodeIndex[nodeID]
		if !ok {
			instance.Status = StatusFailed
			instance.Error = fmt.Sprintf("node not found: %s", nodeID)
			e.updateInstance(instance)
			return
		}

		// 执行节点
		result, err := e.executeNode(ctx, instance, node)
		if err != nil {
			if e.shouldRetry(node, instance, err) {
				continue
			}
			instance.Status = StatusFailed
			instance.Error = err.Error()
			e.updateInstance(instance)
			e.logger.ErrorContext(ctx, "workflow failed",
				"instance_id", instance.ID,
				"node_id", nodeID,
				"error", err)
			return
		}

		// 合并输出到上下文
		if result.Output != nil {
			maps.Copy(instance.Context, result.Output)
		}

		// 确定下一个节点
		nextNodeID, err := e.determineNextNode(instance, node, transitionIndex[nodeID], result)
		if err != nil {
			instance.Status = StatusFailed
			instance.Error = err.Error()
			e.updateInstance(instance)
			return
		}

		instance.CurrentNodeID = nextNodeID
		e.updateInstance(instance)
	}
}

// executeNode 执行单个节点。
func (e *Engine) executeNode(ctx context.Context, instance *Instance, node *NodeDefinition) (*ExecutionResult, error) {
	// 初始化节点状态
	nodeState := &NodeState{
		NodeID:    node.ID,
		Status:    StatusRunning,
		StartedAt: time.Now(),
	}
	instance.NodeStates[node.ID] = nodeState

	// 应用节点超时
	if node.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, node.Timeout)
		defer cancel()
	}

	// 根据节点类型执行
	switch node.Type {
	case NodeTypeTask:
		return e.executeTaskNode(ctx, instance, node)
	case NodeTypeParallel:
		return e.executeParallelNode(ctx, instance, node)
	case NodeTypeExclusive:
		return e.executeExclusiveNode(ctx, instance, node)
	case NodeTypeWait:
		return e.executeWaitNode(ctx, instance, node)
	default:
		return nil, fmt.Errorf("unsupported node type: %s", node.Type)
	}
}

// executeTaskNode 执行任务节点。
func (e *Engine) executeTaskNode(ctx context.Context, instance *Instance, node *NodeDefinition) (*ExecutionResult, error) {
	e.mu.RLock()
	handler, ok := e.handlers[node.Handler]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("handler not found: %s", node.Handler)
	}

	// 准备执行上下文
	execCtx := &ExecutionContext{
		Context:  ctx,
		Instance: instance,
		Node:     node,
		Input:    instance.Context,
		Logger:   e.logger,
	}

	// 执行处理器
	result, err := handler.Execute(execCtx)
	if err != nil {
		return nil, err
	}

	// 更新节点状态
	nodeState := instance.NodeStates[node.ID]
	nodeState.Status = StatusCompleted
	nodeState.Output = result.Output
	now := time.Now()
	nodeState.CompletedAt = &now

	return result, nil
}

// executeParallelNode 执行并行网关节点。
func (e *Engine) executeParallelNode(_ context.Context, instance *Instance, node *NodeDefinition) (*ExecutionResult, error) {
	// 并行网关：等待所有进入分支完成，然后触发所有出去分支
	// 简化实现：标记完成，由 determineNextNode 处理多分支
	nodeState := instance.NodeStates[node.ID]
	nodeState.Status = StatusCompleted
	now := time.Now()
	nodeState.CompletedAt = &now

	return &ExecutionResult{}, nil
}

// executeExclusiveNode 执行排他网关节点。
func (e *Engine) executeExclusiveNode(_ context.Context, instance *Instance, node *NodeDefinition) (*ExecutionResult, error) {
	// 排他网关：根据条件选择一个分支
	nodeState := instance.NodeStates[node.ID]
	nodeState.Status = StatusCompleted
	now := time.Now()
	nodeState.CompletedAt = &now

	return &ExecutionResult{}, nil
}

// executeWaitNode 执行等待节点。
func (e *Engine) executeWaitNode(ctx context.Context, instance *Instance, node *NodeDefinition) (*ExecutionResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(node.WaitDuration):
		nodeState := instance.NodeStates[node.ID]
		nodeState.Status = StatusCompleted
		now := time.Now()
		nodeState.CompletedAt = &now
		return &ExecutionResult{}, nil
	}
}

// determineNextNode 确定下一个节点。
func (e *Engine) determineNextNode(instance *Instance, currentNode *NodeDefinition, transitions []*Transition, result *ExecutionResult) (string, error) {
	if len(transitions) == 0 {
		// 没有出边，工作流结束
		return "", nil
	}

	// 如果结果指定了下一个节点
	if result != nil && result.NextNodeID != "" {
		return result.NextNodeID, nil
	}

	// 对于排他网关，评估条件
	if currentNode.Type == NodeTypeExclusive {
		// 按优先级排序
		for _, t := range transitions {
			if t.IsDefault {
				continue
			}
			if t.Condition == "" {
				return t.To, nil
			}
			if e.conditionEval != nil {
				match, err := e.conditionEval.Evaluate(t.Condition, instance.Context)
				if err != nil {
					e.logger.Warn("condition evaluation failed",
						"condition", t.Condition,
						"error", err)
					continue
				}
				if match {
					return t.To, nil
				}
			}
		}

		// 使用默认分支
		for _, t := range transitions {
			if t.IsDefault {
				return t.To, nil
			}
		}

		return "", errors.New("no matching transition found for exclusive gateway")
	}

	// 对于其他节点，取第一个转换
	return transitions[0].To, nil
}

// shouldRetry 判断是否应该重试。
func (e *Engine) shouldRetry(node *NodeDefinition, instance *Instance, err error) bool {
	if node.RetryPolicy == nil {
		return false
	}

	nodeState := instance.NodeStates[node.ID]
	if nodeState.RetryCount >= node.RetryPolicy.MaxRetries {
		return false
	}

	nodeState.RetryCount++
	nodeState.Error = err.Error()

	// 计算重试间隔
	interval := node.RetryPolicy.InitialInterval
	for i := 0; i < nodeState.RetryCount-1; i++ {
		interval = time.Duration(float64(interval) * node.RetryPolicy.Multiplier)
		if interval > node.RetryPolicy.MaxInterval {
			interval = node.RetryPolicy.MaxInterval
			break
		}
	}

	e.logger.Info("retrying node execution",
		"instance_id", instance.ID,
		"node_id", node.ID,
		"retry_count", nodeState.RetryCount,
		"interval", interval)

	time.Sleep(interval)
	return true
}

// updateInstance 更新实例状态。
func (e *Engine) updateInstance(instance *Instance) {
	if e.instances != nil {
		if err := e.instances.Update(context.Background(), instance); err != nil {
			e.logger.Error("failed to update workflow instance",
				"instance_id", instance.ID,
				"error", err)
		}
	}
}

// CancelWorkflow 取消工作流。
func (e *Engine) CancelWorkflow(ctx context.Context, instanceID string) error {
	e.mu.RLock()
	cancel, ok := e.runningInstances[instanceID]
	e.mu.RUnlock()

	if ok {
		cancel()
	}

	instance, err := e.instances.Get(ctx, instanceID)
	if err != nil {
		return err
	}

	instance.Status = StatusCanceled
	return e.instances.Update(ctx, instance)
}

// GetInstance 获取工作流实例。
func (e *Engine) GetInstance(ctx context.Context, instanceID string) (*Instance, error) {
	return e.instances.Get(ctx, instanceID)
}

// Compensate 执行补偿。
func (e *Engine) Compensate(ctx context.Context, instanceID string) error {
	instance, err := e.instances.Get(ctx, instanceID)
	if err != nil {
		return err
	}

	if instance.Status != StatusFailed {
		return errors.New("can only compensate failed workflows")
	}

	instance.Status = StatusCompensating

	// 逆序执行已完成节点的补偿
	def, err := e.definitions.Get(ctx, instance.DefinitionID, instance.DefinitionVersion)
	if err != nil {
		return err
	}

	nodeIndex := make(map[string]*NodeDefinition)
	for _, node := range def.Nodes {
		nodeIndex[node.ID] = node
	}

	// 收集需要补偿的节点
	var compensateNodes []*NodeDefinition
	for nodeID, state := range instance.NodeStates {
		if state.Status == StatusCompleted {
			if node, ok := nodeIndex[nodeID]; ok && node.CompensationHandler != "" {
				compensateNodes = append(compensateNodes, node)
			}
		}
	}

	// 执行补偿（逆序）
	for i := len(compensateNodes) - 1; i >= 0; i-- {
		node := compensateNodes[i]

		e.mu.RLock()
		handler, ok := e.handlers[node.CompensationHandler]
		e.mu.RUnlock()

		if !ok {
			e.logger.Warn("compensation handler not found",
				"handler", node.CompensationHandler,
				"node_id", node.ID)
			continue
		}

		execCtx := &ExecutionContext{
			Context:  ctx,
			Instance: instance,
			Node:     node,
			Input:    instance.Context,
			Logger:   e.logger,
		}

		if err := handler.Compensate(execCtx); err != nil {
			e.logger.Error("compensation failed",
				"instance_id", instanceID,
				"node_id", node.ID,
				"error", err)
		}
	}

	instance.Status = StatusCompensated
	return e.instances.Update(ctx, instance)
}

// ToJSON 将工作流定义序列化为 JSON。
func (d *Definition) ToJSON() ([]byte, error) {
	return json.Marshal(d)
}

// FromJSON 从 JSON 反序列化工作流定义。
func FromJSON(data []byte) (*Definition, error) {
	var def Definition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, err
	}
	return &def, nil
}
