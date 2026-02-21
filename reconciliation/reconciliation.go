// 变更说明：
// 新增通用对账框架，支持支付对账、清算对账、库存对账等多种场景。
// 核心设计：双数据源比对 + 差异分类 + 自动/人工处理策略。
package reconciliation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// RecordType 对账记录类型。
type RecordType string

const (
	// RecordTypePayment 支付对账。
	RecordTypePayment RecordType = "PAYMENT"
	// RecordTypeSettlement 清算对账。
	RecordTypeSettlement RecordType = "SETTLEMENT"
	// RecordTypeInventory 库存对账。
	RecordTypeInventory RecordType = "INVENTORY"
	// RecordTypeLedger 账务对账。
	RecordTypeLedger RecordType = "LEDGER"
)

// DiffType 差异类型。
type DiffType string

const (
	// DiffTypeMatch 完全匹配。
	DiffTypeMatch DiffType = "MATCH"
	// DiffTypeMissLocal 本地缺失（对方有、本地无）。
	DiffTypeMissLocal DiffType = "MISS_LOCAL"
	// DiffTypeMissRemote 远端缺失（本地有、对方无）。
	DiffTypeMissRemote DiffType = "MISS_REMOTE"
	// DiffTypeAmountMismatch 金额不一致。
	DiffTypeAmountMismatch DiffType = "AMOUNT_MISMATCH"
	// DiffTypeStatusMismatch 状态不一致。
	DiffTypeStatusMismatch DiffType = "STATUS_MISMATCH"
	// DiffTypeFieldMismatch 其他字段不一致。
	DiffTypeFieldMismatch DiffType = "FIELD_MISMATCH"
)

// DiffStatus 差异处理状态。
type DiffStatus string

const (
	// DiffStatusPending 待处理。
	DiffStatusPending DiffStatus = "PENDING"
	// DiffStatusAutoResolved 自动处理完成。
	DiffStatusAutoResolved DiffStatus = "AUTO_RESOLVED"
	// DiffStatusManualPending 等待人工处理。
	DiffStatusManualPending DiffStatus = "MANUAL_PENDING"
	// DiffStatusManualResolved 人工处理完成。
	DiffStatusManualResolved DiffStatus = "MANUAL_RESOLVED"
	// DiffStatusIgnored 已忽略。
	DiffStatusIgnored DiffStatus = "IGNORED"
)

// Record 对账记录，表示一笔可比对的业务数据。
type Record struct {
	// ID 业务唯一标识（如订单号、交易号）。
	ID string `json:"id"`
	// Amount 金额。
	Amount decimal.Decimal `json:"amount"`
	// Status 业务状态。
	Status string `json:"status"`
	// Timestamp 业务发生时间。
	Timestamp time.Time `json:"timestamp"`
	// Extra 扩展字段（用于自定义比对）。
	Extra map[string]string `json:"extra,omitempty"`
}

// DiffRecord 差异记录。
type DiffRecord struct {
	// ID 业务唯一标识。
	ID string `json:"id"`
	// DiffType 差异类型。
	DiffType DiffType `json:"diff_type"`
	// Status 处理状态。
	Status DiffStatus `json:"status"`
	// LocalRecord 本地记录（可能为 nil）。
	LocalRecord *Record `json:"local_record,omitempty"`
	// RemoteRecord 远端记录（可能为 nil）。
	RemoteRecord *Record `json:"remote_record,omitempty"`
	// AmountDiff 金额差异。
	AmountDiff decimal.Decimal `json:"amount_diff"`
	// Details 差异详情描述。
	Details string `json:"details"`
	// ResolvedAt 处理时间。
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	// ResolvedBy 处理人。
	ResolvedBy string `json:"resolved_by,omitempty"`
	// Resolution 处理方案。
	Resolution string `json:"resolution,omitempty"`
	// CreatedAt 发现时间。
	CreatedAt time.Time `json:"created_at"`
}

// Result 对账结果。
type Result struct {
	// BatchID 对账批次 ID。
	BatchID string `json:"batch_id"`
	// RecordType 对账类型。
	RecordType RecordType `json:"record_type"`
	// StartTime 对账区间开始。
	StartTime time.Time `json:"start_time"`
	// EndTime 对账区间结束。
	EndTime time.Time `json:"end_time"`
	// TotalLocal 本地记录总数。
	TotalLocal int `json:"total_local"`
	// TotalRemote 远端记录总数。
	TotalRemote int `json:"total_remote"`
	// MatchCount 匹配数。
	MatchCount int `json:"match_count"`
	// DiffCount 差异数。
	DiffCount int `json:"diff_count"`
	// Diffs 差异记录列表。
	Diffs []*DiffRecord `json:"diffs"`
	// TotalLocalAmount 本地总金额。
	TotalLocalAmount decimal.Decimal `json:"total_local_amount"`
	// TotalRemoteAmount 远端总金额。
	TotalRemoteAmount decimal.Decimal `json:"total_remote_amount"`
	// AmountDiff 总金额差异。
	AmountDiff decimal.Decimal `json:"amount_diff"`
	// Duration 对账耗时。
	Duration time.Duration `json:"duration"`
	// CompletedAt 完成时间。
	CompletedAt time.Time `json:"completed_at"`
}

// DataSource 数据源接口，用于获取对账数据。
type DataSource interface {
	// Fetch 获取指定时间范围内的对账记录。
	Fetch(ctx context.Context, start, end time.Time) ([]*Record, error)
	// Name 数据源名称。
	Name() string
}

// DiffHandler 差异处理器接口。
type DiffHandler interface {
	// Handle 处理差异记录，返回是否已自动处理。
	Handle(ctx context.Context, diff *DiffRecord) (resolved bool, err error)
}

// ResultStore 对账结果存储接口。
type ResultStore interface {
	// SaveResult 保存对账结果。
	SaveResult(ctx context.Context, result *Result) error
	// SaveDiff 保存差异记录。
	SaveDiff(ctx context.Context, diff *DiffRecord) error
	// GetPendingDiffs 获取待处理的差异记录。
	GetPendingDiffs(ctx context.Context, batchID string) ([]*DiffRecord, error)
}

// Comparator 自定义比对函数。
// 返回差异类型和详情描述，DiffTypeMatch 表示匹配。
type Comparator func(local, remote *Record) (DiffType, string)

// Config 对账引擎配置。
type Config struct {
	// BatchID 批次 ID（为空则自动生成）。
	BatchID string
	// RecordType 对账类型。
	RecordType RecordType
	// AmountTolerance 金额容差（小于此值视为匹配）。
	AmountTolerance decimal.Decimal
	// Comparator 自定义比对函数（为 nil 则使用默认比对）。
	Comparator Comparator
	// AutoResolve 是否启用自动处理。
	AutoResolve bool
	// Concurrency 并发处理差异的协程数。
	Concurrency int
}

// Engine 对账引擎。
// 核心流程：拉取双方数据 → 按 ID 索引 → 逐条比对 → 差异分类 → 自动/人工处理。
type Engine struct {
	logger   *slog.Logger
	store    ResultStore
	handlers []DiffHandler
	mu       sync.RWMutex
}

// NewEngine 创建对账引擎。
func NewEngine(logger *slog.Logger) *Engine {
	return &Engine{
		logger:   logger,
		handlers: make([]DiffHandler, 0),
	}
}

// SetStore 设置结果存储。
func (e *Engine) SetStore(store ResultStore) {
	e.store = store
}

// RegisterHandler 注册差异处理器。
func (e *Engine) RegisterHandler(handler DiffHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers = append(e.handlers, handler)
}

// Reconcile 执行对账。
// localSource: 本地数据源（如系统订单表）。
// remoteSource: 远端数据源（如支付渠道账单）。
// start/end: 对账时间范围。
func (e *Engine) Reconcile(
	ctx context.Context,
	localSource, remoteSource DataSource,
	start, end time.Time,
	cfg Config,
) (*Result, error) {
	startTime := time.Now()

	e.logger.Info("reconciliation started",
		"batch_id", cfg.BatchID,
		"type", cfg.RecordType,
		"local_source", localSource.Name(),
		"remote_source", remoteSource.Name(),
		"start", start.Format(time.RFC3339),
		"end", end.Format(time.RFC3339),
	)

	// 1. 并行拉取双方数据
	localRecords, remoteRecords, err := e.fetchBothSources(ctx, localSource, remoteSource, start, end)
	if err != nil {
		return nil, fmt.Errorf("fetch data failed: %w", err)
	}

	// 2. 构建索引
	localIndex := buildIndex(localRecords)
	remoteIndex := buildIndex(remoteRecords)

	// 3. 执行比对
	result := e.compare(localIndex, remoteIndex, localRecords, remoteRecords, cfg)
	result.StartTime = start
	result.EndTime = end
	result.Duration = time.Since(startTime)
	result.CompletedAt = time.Now()

	// 4. 处理差异
	if cfg.AutoResolve && len(result.Diffs) > 0 {
		e.handleDiffs(ctx, result.Diffs, cfg.Concurrency)
	}

	// 5. 持久化结果
	if e.store != nil {
		if saveErr := e.store.SaveResult(ctx, result); saveErr != nil {
			e.logger.Error("save reconciliation result failed", "error", saveErr)
		}
	}

	e.logger.Info("reconciliation completed",
		"batch_id", result.BatchID,
		"total_local", result.TotalLocal,
		"total_remote", result.TotalRemote,
		"match_count", result.MatchCount,
		"diff_count", result.DiffCount,
		"amount_diff", result.AmountDiff.String(),
		"duration", result.Duration.String(),
	)

	return result, nil
}

// fetchBothSources 并行拉取双方数据。
func (e *Engine) fetchBothSources(
	ctx context.Context,
	localSource, remoteSource DataSource,
	start, end time.Time,
) ([]*Record, []*Record, error) {
	type fetchResult struct {
		records []*Record
		err     error
	}

	localCh := make(chan fetchResult, 1)
	remoteCh := make(chan fetchResult, 1)

	go func() {
		records, err := localSource.Fetch(ctx, start, end)
		localCh <- fetchResult{records: records, err: err}
	}()

	go func() {
		records, err := remoteSource.Fetch(ctx, start, end)
		remoteCh <- fetchResult{records: records, err: err}
	}()

	localResult := <-localCh
	remoteResult := <-remoteCh

	if localResult.err != nil {
		return nil, nil, fmt.Errorf("fetch local data failed: %w", localResult.err)
	}
	if remoteResult.err != nil {
		return nil, nil, fmt.Errorf("fetch remote data failed: %w", remoteResult.err)
	}

	return localResult.records, remoteResult.records, nil
}

// compare 执行比对逻辑。
func (e *Engine) compare(
	localIndex, remoteIndex map[string]*Record,
	localRecords, remoteRecords []*Record,
	cfg Config,
) *Result {
	batchID := cfg.BatchID
	if batchID == "" {
		batchID = fmt.Sprintf("RECON-%d", time.Now().UnixMilli())
	}

	result := &Result{
		BatchID:           batchID,
		RecordType:        cfg.RecordType,
		TotalLocal:        len(localRecords),
		TotalRemote:       len(remoteRecords),
		Diffs:             make([]*DiffRecord, 0),
		TotalLocalAmount:  decimal.Zero,
		TotalRemoteAmount: decimal.Zero,
	}

	// 计算总金额
	for _, r := range localRecords {
		result.TotalLocalAmount = result.TotalLocalAmount.Add(r.Amount)
	}
	for _, r := range remoteRecords {
		result.TotalRemoteAmount = result.TotalRemoteAmount.Add(r.Amount)
	}
	result.AmountDiff = result.TotalLocalAmount.Sub(result.TotalRemoteAmount)

	visited := make(map[string]bool)

	// 遍历本地记录，查找远端匹配
	for id, localRecord := range localIndex {
		visited[id] = true
		remoteRecord, exists := remoteIndex[id]

		if !exists {
			result.Diffs = append(result.Diffs, &DiffRecord{
				ID:          id,
				DiffType:    DiffTypeMissRemote,
				Status:      DiffStatusPending,
				LocalRecord: localRecord,
				Details:     "record exists locally but missing in remote",
				CreatedAt:   time.Now(),
			})
			continue
		}

		// 执行比对
		diffType, details := e.compareRecords(localRecord, remoteRecord, cfg)
		if diffType == DiffTypeMatch {
			result.MatchCount++
			continue
		}

		diff := &DiffRecord{
			ID:           id,
			DiffType:     diffType,
			Status:       DiffStatusPending,
			LocalRecord:  localRecord,
			RemoteRecord: remoteRecord,
			AmountDiff:   localRecord.Amount.Sub(remoteRecord.Amount),
			Details:      details,
			CreatedAt:    time.Now(),
		}
		result.Diffs = append(result.Diffs, diff)
	}

	// 遍历远端记录，查找本地缺失
	for id, remoteRecord := range remoteIndex {
		if visited[id] {
			continue
		}
		result.Diffs = append(result.Diffs, &DiffRecord{
			ID:           id,
			DiffType:     DiffTypeMissLocal,
			Status:       DiffStatusPending,
			RemoteRecord: remoteRecord,
			Details:      "record exists in remote but missing locally",
			CreatedAt:    time.Now(),
		})
	}

	result.DiffCount = len(result.Diffs)
	return result
}

// compareRecords 比对两条记录。
func (e *Engine) compareRecords(local, remote *Record, cfg Config) (DiffType, string) {
	// 使用自定义比对器
	if cfg.Comparator != nil {
		return cfg.Comparator(local, remote)
	}

	// 默认比对逻辑
	// 1. 金额比对
	amountDiff := local.Amount.Sub(remote.Amount).Abs()
	tolerance := cfg.AmountTolerance
	if tolerance.IsZero() {
		tolerance = decimal.NewFromFloat(0.01) // 默认容差 0.01
	}
	if amountDiff.GreaterThan(tolerance) {
		return DiffTypeAmountMismatch, fmt.Sprintf(
			"amount mismatch: local=%s, remote=%s, diff=%s",
			local.Amount.String(), remote.Amount.String(), amountDiff.String(),
		)
	}

	// 2. 状态比对
	if local.Status != remote.Status {
		return DiffTypeStatusMismatch, fmt.Sprintf(
			"status mismatch: local=%s, remote=%s",
			local.Status, remote.Status,
		)
	}

	return DiffTypeMatch, ""
}

// handleDiffs 处理差异记录。
func (e *Engine) handleDiffs(ctx context.Context, diffs []*DiffRecord, concurrency int) {
	e.mu.RLock()
	handlers := make([]DiffHandler, len(e.handlers))
	copy(handlers, e.handlers)
	e.mu.RUnlock()

	if len(handlers) == 0 {
		return
	}

	if concurrency <= 0 {
		concurrency = 1
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, diff := range diffs {
		wg.Add(1)
		sem <- struct{}{}

		go func(d *DiffRecord) {
			defer wg.Done()
			defer func() { <-sem }()

			for _, handler := range handlers {
				resolved, err := handler.Handle(ctx, d)
				if err != nil {
					e.logger.Error("diff handler failed",
						"diff_id", d.ID,
						"diff_type", d.DiffType,
						"error", err,
					)
					continue
				}
				if resolved {
					now := time.Now()
					d.Status = DiffStatusAutoResolved
					d.ResolvedAt = &now
					d.ResolvedBy = "SYSTEM"
					break
				}
			}

			// 未自动处理的标记为等待人工
			if d.Status == DiffStatusPending {
				d.Status = DiffStatusManualPending
			}

			// 持久化差异记录
			if e.store != nil {
				if err := e.store.SaveDiff(ctx, d); err != nil {
					e.logger.Error("save diff record failed",
						"diff_id", d.ID,
						"error", err,
					)
				}
			}
		}(diff)
	}

	wg.Wait()
}

// buildIndex 构建 ID 索引。
func buildIndex(records []*Record) map[string]*Record {
	index := make(map[string]*Record, len(records))
	for _, r := range records {
		index[r.ID] = r
	}
	return index
}

// DefaultConfig 返回默认对账配置。
func DefaultConfig(recordType RecordType) Config {
	return Config{
		RecordType:      recordType,
		AmountTolerance: decimal.NewFromFloat(0.01),
		AutoResolve:     true,
		Concurrency:     4,
	}
}
