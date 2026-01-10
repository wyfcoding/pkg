// Package algorithm 提供高性能/ACM/竞赛算法集合。
package algorithm

import (
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wyfcoding/pkg/idgen"
)

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
	ResultChan chan any        `json:"-"` // 异步撮合结果反馈通道。
	OrderID    string          // 订单 ID。
	Symbol     string          // 交易对名称。
	UserID     string          // 用户唯一标识。
	Price      decimal.Decimal // 委托价格。
	Quantity   decimal.Decimal // 委托数量。
	DisplayQty decimal.Decimal // 冰山单单次可见规模。
	HiddenQty  decimal.Decimal // 冰山单当前隐性规模。
	Timestamp  int64           // 纳秒级时间戳，用于时间优先排序。
	Side       Side            // 方向 (BUY/SELL)。
	IsIceberg  bool            // 是否为冰山单。
	PostOnly   bool            // 是否为只做 Maker 策略。
}

// OrderBookLevel 订单簿档位，聚合了同一价格下的委托总量。
type OrderBookLevel struct {
	Price  decimal.Decimal `json:"price"` // 档位价格。
	Qty    decimal.Decimal `json:"quantity"`
	Orders []*Order        `json:"orders"` // 该档位包含的具体订单列表。
}

// OrderBook 订单簿核心结构，管理买卖双边挂单。
type OrderBook struct {
	orders map[string]*RBNode // 订单索引映射。
	bids   *RBTree            // 买单红黑树（价格从高到低）。
	asks   *RBTree            // 卖单红黑树（价格从低到高）。
	mu     sync.RWMutex
}

// NewOrderBook 创建订单簿。
func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids:   NewRBTree(true),  // 买单：价格高的优先。
		asks:   NewRBTree(false), // 卖单：价格低的优先。
		orders: make(map[string]*RBNode),
	}
}

// AddOrder 添加订单。
func (ob *OrderBook) AddOrder(order *Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	var node *RBNode
	if order.Side == SideBuy {
		node = ob.bids.Insert(order)
	} else {
		node = ob.asks.Insert(order)
	}

	ob.orders[order.OrderID] = node
}

// RemoveOrder 移除订单。
func (ob *OrderBook) RemoveOrder(orderID string) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	node, ok := ob.orders[orderID]
	if !ok {
		return
	}

	delete(ob.orders, orderID)

	if node.Order.Side == SideBuy {
		ob.bids.DeleteNode(node)
	} else {
		ob.asks.DeleteNode(node)
	}
}

// GetBestBid 获取最优买价。
func (ob *OrderBook) GetBestBid() *Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.bids.GetBest()
}

// GetBestAsk 获取最优卖价。
func (ob *OrderBook) GetBestAsk() *Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.asks.GetBest()
}

// GetBids 获取买单列表。
func (ob *OrderBook) GetBids(depth int) []*OrderBookLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.getLevels(ob.bids, depth)
}

// GetAsks 获取卖单列表。
func (ob *OrderBook) GetAsks(depth int) []*OrderBookLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.getLevels(ob.asks, depth)
}

func (ob *OrderBook) getLevels(tree *RBTree, depth int) []*OrderBookLevel {
	levels := make([]*OrderBookLevel, 0)
	if tree.Root == nil {
		return levels
	}

	it := tree.NewIterator()

	var currentLevel *OrderBookLevel

	for {
		order := it.Next()
		if order == nil {
			break
		}

		if currentLevel == nil || !currentLevel.Price.Equal(order.Price) {
			if len(levels) >= depth {
				break
			}

			currentLevel = &OrderBookLevel{
				Price:  order.Price,
				Qty:    decimal.Zero,
				Orders: make([]*Order, 0),
			}
			levels = append(levels, currentLevel)
		}

		currentLevel.Qty = currentLevel.Qty.Add(order.Quantity)
		currentLevel.Orders = append(currentLevel.Orders, order)
	}

	return levels
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

// MatchingEngine 撮合引擎。
type MatchingEngine struct {
	orderBook *OrderBook
	trades    []*Trade
	mu        sync.RWMutex
}

var (
	orderPool = sync.Pool{
		New: func() any {
			return &Order{}
		},
	}
	tradePool = sync.Pool{
		New: func() any {
			return &Trade{}
		},
	}
)

// AcquireOrder 从对象池获取 Order。
func AcquireOrder() *Order {
	val, ok := orderPool.Get().(*Order)
	if !ok {
		return &Order{}
	}

	return val
}

// ReleaseOrder 将 Order 放回对象池。
func ReleaseOrder(o *Order) {
	*o = Order{} // 重置对象。
	orderPool.Put(o)
}

// AcquireTrade 从对象池获取 Trade。
func AcquireTrade() *Trade {
	val, ok := tradePool.Get().(*Trade)
	if !ok {
		return &Trade{}
	}

	return val
}

// ReleaseTrade 将 Trade 放回对象池。
func ReleaseTrade(t *Trade) {
	*t = Trade{} // 重置对象。
	tradePool.Put(t)
}

// NewMatchingEngine 创建撮合引擎。
func NewMatchingEngine() *MatchingEngine {
	return &MatchingEngine{
		orderBook: NewOrderBook(),
		trades:    make([]*Trade, 0),
	}
}

// Match 撮合订单。
func (me *MatchingEngine) Match(order *Order) []*Trade {
	me.mu.Lock()
	defer me.mu.Unlock()

	trades := make([]*Trade, 0)

	if order.PostOnly && me.wouldMatch(order) {
		return trades
	}

	var sideTree *RBTree
	if order.Side == SideBuy {
		sideTree = me.orderBook.asks
	} else {
		sideTree = me.orderBook.bids
	}

	for order.Quantity.GreaterThan(decimal.Zero) {
		bestCounter := sideTree.GetBest()
		if bestCounter == nil || !me.canMatch(order, bestCounter) {
			break
		}

		matchQty := decimal.Min(order.Quantity, bestCounter.Quantity)
		trade := me.executeTrade(order, bestCounter, matchQty)
		trades = append(trades, trade)

		order.Quantity = order.Quantity.Sub(matchQty)
		bestCounter.Quantity = bestCounter.Quantity.Sub(matchQty)

		if bestCounter.Quantity.Equal(decimal.Zero) {
			me.handleFinishedOrder(bestCounter)
		}
	}

	if order.Quantity.GreaterThan(decimal.Zero) {
		me.enqueueRemaining(order)
	}

	me.trades = append(me.trades, trades...)

	return trades
}

func (me *MatchingEngine) wouldMatch(order *Order) bool {
	if order.Side == SideBuy {
		bestAsk := me.orderBook.GetBestAsk()

		return bestAsk != nil && bestAsk.Price.LessThanOrEqual(order.Price)
	}

	bestBid := me.orderBook.GetBestBid()

	return bestBid != nil && bestBid.Price.GreaterThanOrEqual(order.Price)
}

func (me *MatchingEngine) canMatch(order, counter *Order) bool {
	if order.Side == SideBuy {
		return counter.Price.LessThanOrEqual(order.Price)
	}

	return counter.Price.GreaterThanOrEqual(order.Price)
}

func (me *MatchingEngine) executeTrade(taker, maker *Order, qty decimal.Decimal) *Trade {
	t := AcquireTrade()
	t.TradeID = generateTradeID()
	t.Symbol = taker.Symbol
	t.Quantity = qty
	t.Price = maker.Price
	t.Timestamp = time.Now().UnixNano()

	if taker.Side == SideBuy {
		t.BuyOrderID, t.BuyUserID = taker.OrderID, taker.UserID
		t.SellOrderID, t.SellUserID = maker.OrderID, maker.UserID
	} else {
		t.BuyOrderID, t.BuyUserID = maker.OrderID, maker.UserID
		t.SellOrderID, t.SellUserID = taker.OrderID, taker.UserID
	}

	return t
}

func (me *MatchingEngine) handleFinishedOrder(o *Order) {
	if o.IsIceberg && o.HiddenQty.GreaterThan(decimal.Zero) {
		refreshQty := decimal.Min(o.DisplayQty, o.HiddenQty)
		o.Quantity = refreshQty
		o.HiddenQty = o.HiddenQty.Sub(refreshQty)
		o.Timestamp = time.Now().UnixNano()

		me.orderBook.RemoveOrder(o.OrderID)
		me.orderBook.AddOrder(o)
	} else {
		me.orderBook.RemoveOrder(o.OrderID)
	}
}

func (me *MatchingEngine) enqueueRemaining(o *Order) {
	if o.IsIceberg && o.Quantity.GreaterThan(o.DisplayQty) {
		o.HiddenQty = o.Quantity.Sub(o.DisplayQty)
		o.Quantity = o.DisplayQty
	}

	me.orderBook.AddOrder(o)
}

// GetTrades 获取所有交易记录。
func (me *MatchingEngine) GetTrades() []*Trade {
	me.mu.RLock()
	defer me.mu.RUnlock()

	result := make([]*Trade, len(me.trades))
	copy(result, me.trades)

	return result
}

// GetBids 获取买单列表。
func (me *MatchingEngine) GetBids(depth int) []*OrderBookLevel {
	me.mu.RLock()
	defer me.mu.RUnlock()

	return me.orderBook.GetBids(depth)
}

// GetAsks 获取卖单列表。
func (me *MatchingEngine) GetAsks(depth int) []*OrderBookLevel {
	me.mu.RLock()
	defer me.mu.RUnlock()

	return me.orderBook.GetAsks(depth)
}

func generateTradeID() string {
	id := idgen.GenID()
	return "TRD" + strconv.FormatUint(id, 10)
}
