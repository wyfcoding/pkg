// Package algos 提供高性能数据结构和算.
package algorithm

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ===== 撮合引擎相关数据结构 ====.

// OrderBookSnapshot 订单簿快.
type OrderBookSnapshot struct {
	Symbol    string
	Bids      []*PriceLevel
	Asks      []*PriceLevel
	Timestamp int64
}

// PriceLevel 价格层.
type PriceLevel struct {
	Price    decimal.Decimal
	Quantity decimal.Decimal
	Count    int64 // 订单数.
}

// ===== 风险管理相关数据结构 ====.

// RiskMetricsSnapshot 风险指标快.
type RiskMetricsSnapshot struct {
	VaR95          decimal.Decimal
	VaR99          decimal.Decimal
	MaxDrawdown    decimal.Decimal
	SharpeRatio    decimal.Decimal
	Correlation    decimal.Decimal
	PortfolioValue decimal.Decimal
	DailyLoss      decimal.Decimal
	Leverage       decimal.Decimal
	UserID         string
	Timestamp      int64
}

// ===== 持仓管理相关数据结构 ====.

// PositionSnapshot 持仓快.
type PositionSnapshot struct {
	Quantity      decimal.Decimal
	EntryPrice    decimal.Decimal
	CurrentPrice  decimal.Decimal
	UnrealizedPnL decimal.Decimal
	RealizedPnL   decimal.Decimal
	Leverage      decimal.Decimal
	UserID        string
	Symbol        string
	Side          string
	Timestamp     int64
}

// PortfolioSnapshot 投资组合快.
type PortfolioSnapshot struct {
	UserID             string
	Positions          []*PositionSnapshot
	TotalValue         decimal.Decimal
	CashBalance        decimal.Decimal
	TotalUnrealizedPnL decimal.Decimal
	TotalRealizedPnL   decimal.Decimal
	Timestamp          int64
}

// ===== 清算相关数据结构 ====.

// SettlementBatch 清算批.
type SettlementBatch struct {
	CompletedAt *int64
	BatchID     string
	Status      string
	Trades      []*TradeSettlement
	CreatedAt   int64
}

// TradeSettlement 交易清算记.
type TradeSettlement struct {
	Quantity   decimal.Decimal
	Price      decimal.Decimal
	TradeID    string
	BuyUserID  string
	SellUserID string
	Symbol     string
	Status     string
	SettledAt  int64
}

// ===== 定价相关数据结构 ====.

// OptionPricingModel 期权定价模.
type OptionPricingModel struct {
	UnderlyingPrice decimal.Decimal
	StrikePrice     decimal.Decimal
	TimeToExpiry    decimal.Decimal // 年.
	RiskFreeRate    decimal.Decimal
	Volatility      decimal.Decimal
	DividendYield   decimal.Decimal
	OptionType      string // CALL or PU.
}

// OptionPricingResult 期权定价结.
type OptionPricingResult struct {
	OptionPrice    decimal.Decimal
	IntrinsicValue decimal.Decimal
	TimeValue      decimal.Decimal
	Delta          decimal.Decimal
	Gamma          decimal.Decimal
	Vega           decimal.Decimal
	Theta          decimal.Decimal
	Rho            decimal.Decimal
}

// ===== 做市相关数据结构 ====.

// MarketMakingQuote 做市报.
type MarketMakingQuote struct {
	BidPrice    decimal.Decimal
	AskPrice    decimal.Decimal
	BidQuantity decimal.Decimal
	AskQuantity decimal.Decimal
	Spread      decimal.Decimal
	Symbol      string
	Timestamp   int64
}

// MarketMakingMetrics 做市指.
type MarketMakingMetrics struct {
	TotalPnL      decimal.Decimal
	DailyPnL      decimal.Decimal
	SpreadEarned  decimal.Decimal
	WinRate       decimal.Decimal
	AverageSpread decimal.Decimal
	MarketMakerID string
	Symbol        string
	TradesCount   int64
	Timestamp     int64
}

// ===== 量化相关数据结构 ====.

// StrategySignal 策略信.
type StrategySignal struct {
	Confidence  decimal.Decimal
	TargetPrice decimal.Decimal
	StopLoss    decimal.Decimal
	TakeProfit  decimal.Decimal
	StrategyID  string
	Symbol      string
	Signal      string // BUY, SELL, HOL.
	Timestamp   int64
}

// BacktestMetrics 回测指.
type BacktestMetrics struct {
	TotalReturn   decimal.Decimal
	AnnualReturn  decimal.Decimal
	SharpeRatio   decimal.Decimal
	MaxDrawdown   decimal.Decimal
	WinRate       decimal.Decimal
	ProfitFactor  decimal.Decimal
	TradesCount   int64
	WinningTrades int64
	LosingTrades  int64
}

// ===== 市场模拟相关数据结构 ====.

// PriceBar K线数.
type PriceBar struct {
	Open      decimal.Decimal
	High      decimal.Decimal
	Low       decimal.Decimal
	Close     decimal.Decimal
	Volume    decimal.Decimal
	Timestamp int64
}

// SimulationState 模拟状.
type SimulationState struct {
	CurrentPrice decimal.Decimal
	MinPrice     decimal.Decimal
	MaxPrice     decimal.Decimal
	Volume       decimal.Decimal
	Volatility   decimal.Decimal
	Timestamp    int64
}

// ===== 监控分析相关数据结构 ====.

// SystemMetricsData 系统指标数.
type SystemMetricsData struct {
	CPUUsage          decimal.Decimal
	MemoryUsage       decimal.Decimal
	DiskUsage         decimal.Decimal
	NetworkThroughput decimal.Decimal
	AverageLatency    decimal.Decimal
	ErrorRate         decimal.Decimal
	Timestamp         int64
	ActiveConnections int64
	RequestsPerSecond int64
}

// TradeStatisticsData 交易统计数.
type TradeStatisticsData struct {
	TotalVolume  decimal.Decimal
	TotalValue   decimal.Decimal
	AveragePrice decimal.Decimal
	HighPrice    decimal.Decimal
	LowPrice     decimal.Decimal
	OpenPrice    decimal.Decimal
	ClosePrice   decimal.Decimal
	Symbol       string
	TotalTrades  int64
	Timestamp    int64
}

// UserAnalyticsData 用户分析数.
type UserAnalyticsData struct {
	TotalPnL         decimal.Decimal
	WinRate          decimal.Decimal
	AverageTradeSize decimal.Decimal
	MaxDrawdown      decimal.Decimal
	SharpeRatio      decimal.Decimal
	ProfitFactor     decimal.Decimal
	UserID           string
	TotalTrades      int64
	Timestamp        int64
}

// ===== 参考数据相关数据结构 ====.

// SymbolInfo 交易对信.
type SymbolInfo struct {
	MinPrice    decimal.Decimal
	MaxPrice    decimal.Decimal
	MinQuantity decimal.Decimal
	MaxQuantity decimal.Decimal
	TickSize    decimal.Decimal
	StepSize    decimal.Decimal
	MakerFee    decimal.Decimal
	TakerFee    decimal.Decimal
	Symbol      string
	BaseAsset   string
	QuoteAsset  string
	Status      string
	UpdatedAt   int64
}

// ExchangeInfo 交易所信.
type ExchangeInfo struct {
	Name             string
	Timezone         string
	TradingDays      []string
	TradingStartTime int64
	TradingEndTime   int64
	UpdatedAt        int64
}

// AssetInfo 资产信.
type AssetInfo struct {
	MinWithdrawal   decimal.Decimal
	WithdrawalFee   decimal.Decimal
	Asset           string
	Name            string
	UpdatedAt       int64
	Decimals        int32
	DepositEnabled  bool
	WithdrawEnabled bool
}

// ===== 通知相关数据结构 ====.

// NotificationMessage 通知消.
type NotificationMessage struct {
	Metadata       map[string]string
	ReadAt         *int64
	NotificationID string
	UserID         string
	Type           string // ORDER, TRADE, RISK, SYSTE.
	Title          string
	Content        string
	CreatedAt      int64
	IsRead         bool
}

// ===== 线程安全的缓存结构 ====.

// ConcurrentCache 并发安全的缓.
type ConcurrentCache struct {
	data     map[string]any
	expireAt map[string]int64 // 过期时间戳 (Unix Seconds.
	mu       sync.RWMutex
}

// NewConcurrentCache 创建并发缓.
func NewConcurrentCache() *ConcurrentCache {
	return &ConcurrentCache{
		data:     make(map[string]any),
		expireAt: make(map[string]int64),
	}
}

// Set 设置缓存.
// expireAt: 过期时间戳 (Unix Seconds)。如果为 0，表示永不过期。
func (cc *ConcurrentCache) Set(key string, value any, expireAt int64) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.data[key] = value
	cc.expireAt[key] = expireAt
}

// Get 获取缓存.
func (cc *ConcurrentCache) Get(key string) (any, bool) {
	cc.mu.RLock()
	expiry, hasExpiry := cc.expireAt[key]
	val, ok := cc.data[key]
	cc.mu.RUnlock()

	if !ok {
		return nil, false
	}

	// 检查过期 (Lazy Expiration.
	if hasExpiry && expiry > 0 && time.Now().Unix() > expiry {
		// 升级锁进行删.
		cc.mu.Lock()
		// 双重检.
		if exp, exists := cc.expireAt[key]; exists && exp > 0 && time.Now().Unix() > exp {
			delete(cc.data, key)
			delete(cc.expireAt, key)
		}
		cc.mu.Unlock()
		return nil, false
	}

	return val, true
}

// Delete 删除缓存.
func (cc *ConcurrentCache) Delete(key string) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	delete(cc.data, key)
	delete(cc.expireAt, key)
}

// Clear 清空缓.
func (cc *ConcurrentCache) Clear() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.data = make(map[string]any)
	cc.expireAt = make(map[string]int64)
}
