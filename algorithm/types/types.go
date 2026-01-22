package types

import "github.com/shopspring/decimal"

// Side 定义交易方向。
type Side string

const (
	// SideBuy 买入。
	SideBuy Side = "BUY"
	// SideSell 卖出。
	SideSell Side = "SELL"
)

// Order 订单结构，表示一个限价单或高级策略单。
type Order struct {
	ResultChan chan any
	Price      decimal.Decimal
	Quantity   decimal.Decimal
	DisplayQty decimal.Decimal
	HiddenQty  decimal.Decimal
	PegOffset  decimal.Decimal // 偏移量
	OrderID    string
	Symbol     string
	UserID     string
	PegType    string // "MID", "BEST_BID", "BEST_ASK"
	Timestamp  int64
	Side       Side
	IsIceberg  bool
	PostOnly   bool
	IsPegged   bool
}

// OrderBookLevel 订单簿档位，聚合了同一价格下的委托总量。
type OrderBookLevel struct {
	Price  decimal.Decimal `json:"price"` // 档位价格。
	Qty    decimal.Decimal `json:"quantity"`
	Orders []*Order        `json:"orders"` // 该档位包含的具体订单列表。
}

// Trade 交易记录。
type Trade struct {
	Price       decimal.Decimal
	Quantity    decimal.Decimal
	TradeID     string
	Symbol      string
	BuyOrderID  string
	SellOrderID string
	BuyUserID   string
	SellUserID  string
	Timestamp   int64
}

// OptionType 定义期权类型。
type OptionType string

const (
	// OptionTypeCall 看涨期权。
	OptionTypeCall OptionType = "CALL"
	// OptionTypePut 看跌期权。
	OptionTypePut OptionType = "PUT"
)
