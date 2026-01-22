// Package trading 提供了金融交易相关的标准模型与快照结构.
package trading

import (
	"github.com/shopspring/decimal"
)

// OrderBookSnapshot 订单簿深度快照.
type OrderBookSnapshot struct {
	Symbol    string        `json:"symbol"`
	Bids      []*PriceLevel `json:"bids"`
	Asks      []*PriceLevel `json:"asks"`
	Timestamp int64         `json:"timestamp"`
}

// PriceLevel 价格档位信息.
type PriceLevel struct {
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
	Count    int64           `json:"count"` // 挂单数量
}

// RiskMetricsSnapshot 风险指标快照.
type RiskMetricsSnapshot struct {
	VaR95          decimal.Decimal `json:"var_95"`
	VaR99          decimal.Decimal `json:"var_99"`
	MaxDrawdown    decimal.Decimal `json:"max_drawdown"`
	SharpeRatio    decimal.Decimal `json:"sharpe_ratio"`
	Correlation    decimal.Decimal `json:"correlation"`
	PortfolioValue decimal.Decimal `json:"portfolio_value"`
	DailyLoss      decimal.Decimal `json:"daily_loss"`
	Leverage       decimal.Decimal `json:"leverage"`
	UserID         string          `json:"user_id"`
	Timestamp      int64           `json:"timestamp"`
}

// PositionSnapshot 持仓快照.
type PositionSnapshot struct {
	Quantity      decimal.Decimal `json:"quantity"`
	EntryPrice    decimal.Decimal `json:"entry_price"`
	CurrentPrice  decimal.Decimal `json:"current_price"`
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
	RealizedPnL   decimal.Decimal `json:"realized_pnl"`
	Leverage      decimal.Decimal `json:"leverage"`
	UserID        string          `json:"user_id"`
	Symbol        string          `json:"symbol"`
	Side          string          `json:"side"` // BUY/SELL
	Timestamp     int64           `json:"timestamp"`
}

// PortfolioSnapshot 投资组合全量快照.
type PortfolioSnapshot struct {
	TotalValue         decimal.Decimal     `json:"total_value"`
	CashBalance        decimal.Decimal     `json:"cash_balance"`
	TotalUnrealizedPnL decimal.Decimal     `json:"total_unrealized_pnl"`
	TotalRealizedPnL   decimal.Decimal     `json:"total_realized_pnl"`
	UserID             string              `json:"user_id"`
	Positions          []*PositionSnapshot `json:"positions"`
	Timestamp          int64               `json:"timestamp"`
}

// SettlementBatch 清算批次记录.
type SettlementBatch struct {
	CompletedAt *int64             `json:"completed_at,omitempty"`
	BatchID     string             `json:"batch_id"`
	Status      string             `json:"status"` // PENDING/COMPLETED
	Trades      []*TradeSettlement `json:"trades"`
	CreatedAt   int64              `json:"created_at"`
}

// TradeSettlement 交易清算明细.
type TradeSettlement struct {
	Quantity   decimal.Decimal `json:"quantity"`
	Price      decimal.Decimal `json:"price"`
	TradeID    string          `json:"trade_id"`
	BuyUserID  string          `json:"buy_user_id"`
	SellUserID string          `json:"sell_user_id"`
	Symbol     string          `json:"symbol"`
	Status     string          `json:"status"`
	SettledAt  int64           `json:"settled_at"`
}

// MarketMakingQuote 做市商报价.
type MarketMakingQuote struct {
	BidPrice    decimal.Decimal `json:"bid_price"`
	AskPrice    decimal.Decimal `json:"ask_price"`
	BidQuantity decimal.Decimal `json:"bid_quantity"`
	AskQuantity decimal.Decimal `json:"ask_quantity"`
	Spread      decimal.Decimal `json:"spread"`
	Symbol      string          `json:"symbol"`
	Timestamp   int64           `json:"timestamp"`
}

// StrategySignal 量化策略信号.
type StrategySignal struct {
	Confidence  decimal.Decimal `json:"confidence"`
	TargetPrice decimal.Decimal `json:"target_price"`
	StopLoss    decimal.Decimal `json:"stop_loss"`
	TakeProfit  decimal.Decimal `json:"take_profit"`
	StrategyID  string          `json:"strategy_id"`
	Symbol      string          `json:"symbol"`
	Signal      string          `json:"signal"` // BUY/SELL/HOLD
	Timestamp   int64           `json:"timestamp"`
}

// PriceBar K线数据单元 (OHLCV).
type PriceBar struct {
	Open      decimal.Decimal `json:"open"`
	High      decimal.Decimal `json:"high"`
	Low       decimal.Decimal `json:"low"`
	Close     decimal.Decimal `json:"close"`
	Volume    decimal.Decimal `json:"volume"`
	Timestamp int64           `json:"timestamp"`
}
