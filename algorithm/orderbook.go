// Package algos 提供高性能/ACM/竞赛算法集合，包括撮合相关结构、图算法、DP、后缀结构、并发数据结构等
package algorithm

import (
	"sync"

	"github.com/shopspring/decimal"
)

// Order 订单结构
type Order struct {
	// 订单 ID
	OrderID string
	// 交易对
	Symbol string
	// 买卖方向：BUY 或 SELL
	Side string
	// 价格
	Price decimal.Decimal
	// 数量
	Quantity decimal.Decimal
	// 时间戳（用于时间优先）
	Timestamp int64
	// 用户 ID
	UserID string
}

// OrderBookLevel 订单簿层级
type OrderBookLevel struct {
	// 价格
	Price decimal.Decimal
	// 总数量
	Quantity decimal.Decimal
	// 订单列表
	Orders []*Order
}

// OrderBook 订单簿（支持价格优先、时间优先）
type OrderBook struct {
	mu sync.RWMutex
	// 买单（价格从高到低）
	bids *RBTree
	// 卖单（价格从低到高）
	asks *RBTree
	// 订单映射（用于快速查找）
	orders map[string]*Order
}

// NewOrderBook 创建订单簿
func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids:   NewRBTree(true),  // 买单：价格高的优先
		asks:   NewRBTree(false), // 卖单：价格低的优先
		orders: make(map[string]*Order),
	}
}

// AddOrder 添加订单
func (ob *OrderBook) AddOrder(order *Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	ob.orders[order.OrderID] = order

	if order.Side == "BUY" {
		ob.bids.Insert(order)
	} else {
		ob.asks.Insert(order)
	}
}

// RemoveOrder 移除订单
func (ob *OrderBook) RemoveOrder(orderID string) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	order, ok := ob.orders[orderID]
	if !ok {
		return
	}

	delete(ob.orders, orderID)

	if order.Side == "BUY" {
		ob.bids.Delete(order)
	} else {
		ob.asks.Delete(order)
	}
}

// GetBestBid 获取最优买价
func (ob *OrderBook) GetBestBid() *Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.bids.GetBest()
}

// GetBestAsk 获取最优卖价
func (ob *OrderBook) GetBestAsk() *Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.asks.GetBest()
}

// GetBids 获取买单列表（按价格从高到低）
func (ob *OrderBook) GetBids(depth int) []*OrderBookLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.getLevels(ob.bids, depth)
}

// GetAsks 获取卖单列表（按价格从低到高）
func (ob *OrderBook) GetAsks(depth int) []*OrderBookLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.getLevels(ob.asks, depth)
}

// getLevels 获取订单簿层级
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
				Price:    order.Price,
				Quantity: decimal.Zero,
				Orders:   make([]*Order, 0),
			}
			levels = append(levels, currentLevel)
		}

		currentLevel.Quantity = currentLevel.Quantity.Add(order.Quantity)
		currentLevel.Orders = append(currentLevel.Orders, order)
	}

	return levels
}

// MatchingEngine 撮合引擎（价格优先、时间优先）
type MatchingEngine struct {
	mu        sync.RWMutex
	orderBook *OrderBook
	trades    []*Trade
}

// Trade 交易记录
type Trade struct {
	// 交易 ID
	TradeID string
	// 交易对
	Symbol string
	// 买方订单 ID
	BuyOrderID string
	// 卖方订单 ID
	SellOrderID string
	// 成交价格
	Price decimal.Decimal
	// 成交数量
	Quantity decimal.Decimal
	// 时间戳
	Timestamp int64
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

// AcquireOrder 从对象池获取 Order
func AcquireOrder() *Order {
	return orderPool.Get().(*Order)
}

// ReleaseOrder 将 Order 放回对象池
func ReleaseOrder(o *Order) {
	*o = Order{} // 重置对象
	orderPool.Put(o)
}

// AcquireTrade 从对象池获取 Trade
func AcquireTrade() *Trade {
	return tradePool.Get().(*Trade)
}

// ReleaseTrade 将 Trade 放回对象池
func ReleaseTrade(t *Trade) {
	*t = Trade{} // 重置对象
	tradePool.Put(t)
}

// NewMatchingEngine 创建撮合引擎
func NewMatchingEngine() *MatchingEngine {
	return &MatchingEngine{
		orderBook: NewOrderBook(),
		trades:    make([]*Trade, 0),
	}
}

// Match 撮合订单
func (me *MatchingEngine) Match(order *Order) []*Trade {
	me.mu.Lock()
	defer me.mu.Unlock()

	trades := make([]*Trade, 0)

	if order.Side == "BUY" {
		// 买单与卖单撮合
		for order.Quantity.GreaterThan(decimal.Zero) {
			bestAsk := me.orderBook.GetBestAsk()
			if bestAsk == nil || bestAsk.Price.GreaterThan(order.Price) {
				break
			}

			// 计算成交数量
			matchQty := order.Quantity
			if bestAsk.Quantity.LessThan(matchQty) {
				matchQty = bestAsk.Quantity
			}

			// 创建交易记录
			trade := &Trade{
				TradeID:     generateTradeID(),
				Symbol:      order.Symbol,
				BuyOrderID:  order.OrderID,
				SellOrderID: bestAsk.OrderID,
				Price:       bestAsk.Price,
				Quantity:    matchQty,
				Timestamp:   order.Timestamp,
			}
			trades = append(trades, trade)

			// 更新订单数量
			order.Quantity = order.Quantity.Sub(matchQty)
			bestAsk.Quantity = bestAsk.Quantity.Sub(matchQty)

			// 如果卖单已完全成交，移除
			if bestAsk.Quantity.Equal(decimal.Zero) {
				me.orderBook.RemoveOrder(bestAsk.OrderID)
			}
		}
	} else {
		// 卖单与买单撮合
		for order.Quantity.GreaterThan(decimal.Zero) {
			bestBid := me.orderBook.GetBestBid()
			if bestBid == nil || bestBid.Price.LessThan(order.Price) {
				break
			}

			// 计算成交数量
			matchQty := order.Quantity
			if bestBid.Quantity.LessThan(matchQty) {
				matchQty = bestBid.Quantity
			}

			// 创建交易记录
			trade := &Trade{
				TradeID:     generateTradeID(),
				Symbol:      order.Symbol,
				BuyOrderID:  bestBid.OrderID,
				SellOrderID: order.OrderID,
				Price:       bestBid.Price,
				Quantity:    matchQty,
				Timestamp:   order.Timestamp,
			}
			trades = append(trades, trade)

			// 更新订单数量
			order.Quantity = order.Quantity.Sub(matchQty)
			bestBid.Quantity = bestBid.Quantity.Sub(matchQty)

			// 如果买单已完全成交，移除
			if bestBid.Quantity.Equal(decimal.Zero) {
				me.orderBook.RemoveOrder(bestBid.OrderID)
			}
		}
	}

	// 如果订单还有剩余，添加到订单簿
	if order.Quantity.GreaterThan(decimal.Zero) {
		me.orderBook.AddOrder(order)
	}

	me.trades = append(me.trades, trades...)
	return trades
}

// GetTrades 获取所有交易记录
func (me *MatchingEngine) GetTrades() []*Trade {
	me.mu.RLock()
	defer me.mu.RUnlock()

	result := make([]*Trade, len(me.trades))
	copy(result, me.trades)
	return result
}

// GetBids 获取买单列表
func (me *MatchingEngine) GetBids(depth int) []*OrderBookLevel {
	me.mu.RLock()
	defer me.mu.RUnlock()
	return me.orderBook.GetBids(depth)
}

// GetAsks 获取卖单列表
func (me *MatchingEngine) GetAsks(depth int) []*OrderBookLevel {
	me.mu.RLock()
	defer me.mu.RUnlock()
	return me.orderBook.GetAsks(depth)
}

// generateTradeID 生成交易 ID
func generateTradeID() string {
	// 实际应用中应使用雪花 ID 或 UUID
	return ""
}
