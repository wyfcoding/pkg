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

// InstrumentType 定义交易品种类型。
type InstrumentType string

const (
	InstSpot   InstrumentType = "SPOT"
	InstPerp   InstrumentType = "PERPETUAL" // 永续合约
	InstOption InstrumentType = "OPTION"    // 期权
	InstFuture InstrumentType = "FUTURE"    // 交割合约
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
	Side       Side
	InstType   InstrumentType
	Timestamp  int64
	IsIceberg  bool
	PostOnly   bool
	IsPegged   bool
	// 高级委托类型
	TimeInForce TimeInForce    // GTC, FOK, FAK
	Condition   OrderCondition // NONE, AON
	// 期权/衍生品特特有
	Strike     decimal.Decimal
	Expiry     int64
	OptionType OptionType
	Leverage   decimal.Decimal
}

// TimeInForce 订单生效时间策略。
type TimeInForce string

const (
	TIFGTC TimeInForce = "GTC" // Good Till Cancel
	TIFFOK TimeInForce = "FOK" // Fill Or Kill
	TIFFAK TimeInForce = "FAK" // Fill And Kill (IOC)
)

// OrderCondition 订单限制条件。
type OrderCondition string

const (
	CondNone OrderCondition = "NONE"
	CondAON  OrderCondition = "AON" // All Or None
)

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
	InstType    InstrumentType
	Timestamp   int64
}

// OptionType 定义期权类型。
type OptionType string

const (
	OptionTypeCall OptionType = "CALL"
	OptionTypePut  OptionType = "PUT"
)
