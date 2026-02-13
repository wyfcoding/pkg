// 变更说明：高性能订单簿组件，支持价格档位管理、订单匹配、快照等功能
package orderbook

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wyfcoding/pkg/algorithm/structures"
)

// OrderSide 订单方向
type OrderSide int8

const (
	SideBuy OrderSide = iota
	SideSell
)

func (s OrderSide) String() string {
	if s == SideBuy {
		return "BUY"
	}
	return "SELL"
}

// OrderType 订单类型
type OrderType int8

const (
	OrderTypeLimit OrderType = iota
	OrderTypeMarket
	OrderTypeStopLimit
	OrderTypeStopMarket
	OrderTypeFOK
	OrderTypeIOC
	OrderTypeAON
)

// OrderStatus 订单状态
type OrderStatus int8

const (
	OrderStatusNew OrderStatus = iota
	OrderStatusPartiallyFilled
	OrderStatusFilled
	OrderStatusCancelled
	OrderStatusRejected
	OrderStatusExpired
)

// Order 订单
type Order struct {
	OrderID      string      `json:"order_id"`
	Symbol       string      `json:"symbol"`
	Side         OrderSide   `json:"side"`
	Type         OrderType   `json:"type"`
	Price        float64     `json:"price"`
	Quantity     int64       `json:"quantity"`
	FilledQty    int64       `json:"filled_qty"`
	RemainingQty int64       `json:"remaining_qty"`
	Status       OrderStatus `json:"status"`
	Timestamp    int64       `json:"timestamp"`
	UserID       uint64      `json:"user_id"`
	StopPrice    float64     `json:"stop_price"`
	TimeInForce  string      `json:"time_in_force"`
	ExpireTime   int64       `json:"expire_time"`
}

// Trade 成交记录
type Trade struct {
	TradeID     string  `json:"trade_id"`
	Symbol      string  `json:"symbol"`
	Price       float64 `json:"price"`
	Quantity    int64   `json:"quantity"`
	BuyOrderID  string  `json:"buy_order_id"`
	SellOrderID string  `json:"sell_order_id"`
	Timestamp   int64   `json:"timestamp"`
}

// PriceLevel 价格档位
type PriceLevel struct {
	Price      float64
	Orders     map[string]*Order
	OrderList  []string
	TotalQty   int64
	OrderCount int
}

// NewPriceLevel 创建价格档位
func NewPriceLevel(price float64) *PriceLevel {
	return &PriceLevel{
		Price:     price,
		Orders:    make(map[string]*Order),
		OrderList: make([]string, 0),
	}
}

// AddOrder 添加订单
func (l *PriceLevel) AddOrder(order *Order) {
	l.Orders[order.OrderID] = order
	l.OrderList = append(l.OrderList, order.OrderID)
	l.TotalQty += order.RemainingQty
	l.OrderCount++
}

// RemoveOrder 移除订单
func (l *PriceLevel) RemoveOrder(orderID string) *Order {
	order, ok := l.Orders[orderID]
	if !ok {
		return nil
	}
	delete(l.Orders, orderID)
	for i, id := range l.OrderList {
		if id == orderID {
			l.OrderList = append(l.OrderList[:i], l.OrderList[i+1:]...)
			break
		}
	}
	l.TotalQty -= order.RemainingQty
	l.OrderCount--
	return order
}

// FirstOrder 获取第一个订单
func (l *PriceLevel) FirstOrder() *Order {
	if len(l.OrderList) == 0 {
		return nil
	}
	return l.Orders[l.OrderList[0]]
}

// UpdateQuantity 更新数量
func (l *PriceLevel) UpdateQuantity(orderID string, delta int64) {
	if order, ok := l.Orders[orderID]; ok {
		l.TotalQty -= delta
		order.RemainingQty -= delta
		order.FilledQty += delta
	}
}

// OrderBook 订单簿
type OrderBook struct {
	symbol       string
	bids         *PriceLevelMap
	asks         *PriceLevelMap
	orders       sync.Map
	mu           sync.RWMutex
	version      atomic.Int64
	lastUpdateID int64
	tradeIDGen   atomic.Int64
}

// PriceLevelMap 价格档位映射（使用负价格实现降序）
type PriceLevelMap struct {
	levels map[float64]*PriceLevel
	prices []float64
	desc   bool
}

// NewPriceLevelMap 创建价格档位映射
func NewPriceLevelMap(desc bool) *PriceLevelMap {
	return &PriceLevelMap{
		levels: make(map[float64]*PriceLevel),
		prices: make([]float64, 0),
		desc:   desc,
	}
}

// GetOrCreate 获取或创建价格档位
func (m *PriceLevelMap) GetOrCreate(price float64) *PriceLevel {
	if level, ok := m.levels[price]; ok {
		return level
	}
	level := NewPriceLevel(price)
	m.levels[price] = level
	m.prices = append(m.prices, price)
	m.sort()
	return level
}

// Get 获取价格档位
func (m *PriceLevelMap) Get(price float64) (*PriceLevel, bool) {
	level, ok := m.levels[price]
	return level, ok
}

// Delete 删除价格档位
func (m *PriceLevelMap) Delete(price float64) {
	delete(m.levels, price)
	for i, p := range m.prices {
		if p == price {
			m.prices = append(m.prices[:i], m.prices[i+1:]...)
			break
		}
	}
}

// First 获取最优价格档位
func (m *PriceLevelMap) First() (*PriceLevel, bool) {
	if len(m.prices) == 0 {
		return nil, false
	}
	return m.levels[m.prices[0]], true
}

// Len 获取档位数量
func (m *PriceLevelMap) Len() int {
	return len(m.prices)
}

// Iterate 遍历
func (m *PriceLevelMap) Iterate(fn func(price float64, level *PriceLevel) bool) {
	for _, price := range m.prices {
		if !fn(price, m.levels[price]) {
			break
		}
	}
}

// sort 排序
func (m *PriceLevelMap) sort() {
	n := len(m.prices)
	for i := 0; i < n-1; i++ {
		for j := i + 1; j < n; j++ {
			if m.desc {
				if m.prices[i] < m.prices[j] {
					m.prices[i], m.prices[j] = m.prices[j], m.prices[i]
				}
			} else {
				if m.prices[i] > m.prices[j] {
					m.prices[i], m.prices[j] = m.prices[j], m.prices[i]
				}
			}
		}
	}
}

// NewOrderBook 创建订单簿
func NewOrderBook(symbol string) *OrderBook {
	ob := &OrderBook{
		symbol: symbol,
		bids:   NewPriceLevelMap(true),
		asks:   NewPriceLevelMap(false),
	}
	return ob
}

// AddOrder 添加订单
func (ob *OrderBook) AddOrder(order *Order) ([]*Trade, error) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	order.Timestamp = time.Now().UnixNano()
	order.Status = OrderStatusNew
	order.RemainingQty = order.Quantity

	if order.Type == OrderTypeMarket {
		return ob.matchMarketOrder(order), nil
	}

	return ob.addLimitOrder(order), nil
}

// addLimitOrder 添加限价单
func (ob *OrderBook) addLimitOrder(order *Order) []*Trade {
	var trades []*Trade

	if order.Side == SideBuy {
		trades = ob.matchBuyOrder(order)
	} else {
		trades = ob.matchSellOrder(order)
	}

	if order.RemainingQty > 0 && order.Type != OrderTypeIOC {
		ob.insertOrder(order)
	}

	ob.version.Add(1)
	ob.lastUpdateID = time.Now().UnixNano()

	return trades
}

// matchBuyOrder 匹配买单
func (ob *OrderBook) matchBuyOrder(order *Order) []*Trade {
	var trades []*Trade

	for ob.asks.Len() > 0 && order.RemainingQty > 0 {
		level, ok := ob.asks.First()
		if !ok || level.Price > order.Price {
			break
		}

		for level.OrderCount > 0 && order.RemainingQty > 0 {
			sellOrder := level.FirstOrder()
			if sellOrder == nil {
				break
			}

			matchQty := min(order.RemainingQty, sellOrder.RemainingQty)
			trade := ob.createTrade(order, sellOrder, matchQty, level.Price)
			trades = append(trades, trade)

			level.UpdateQuantity(sellOrder.OrderID, matchQty)
			order.RemainingQty -= matchQty
			order.FilledQty += matchQty

			if sellOrder.RemainingQty == 0 {
				sellOrder.Status = OrderStatusFilled
				level.RemoveOrder(sellOrder.OrderID)
				ob.orders.Delete(sellOrder.OrderID)
			}
		}

		if level.OrderCount == 0 {
			ob.asks.Delete(level.Price)
		}
	}

	if order.FilledQty > 0 {
		if order.RemainingQty == 0 {
			order.Status = OrderStatusFilled
		} else {
			order.Status = OrderStatusPartiallyFilled
		}
	}

	return trades
}

// matchSellOrder 匹配卖单
func (ob *OrderBook) matchSellOrder(order *Order) []*Trade {
	var trades []*Trade

	for ob.bids.Len() > 0 && order.RemainingQty > 0 {
		level, ok := ob.bids.First()
		if !ok || level.Price < order.Price {
			break
		}

		for level.OrderCount > 0 && order.RemainingQty > 0 {
			buyOrder := level.FirstOrder()
			if buyOrder == nil {
				break
			}

			matchQty := min(order.RemainingQty, buyOrder.RemainingQty)
			trade := ob.createTrade(buyOrder, order, matchQty, level.Price)
			trades = append(trades, trade)

			level.UpdateQuantity(buyOrder.OrderID, matchQty)
			order.RemainingQty -= matchQty
			order.FilledQty += matchQty

			if buyOrder.RemainingQty == 0 {
				buyOrder.Status = OrderStatusFilled
				level.RemoveOrder(buyOrder.OrderID)
				ob.orders.Delete(buyOrder.OrderID)
			}
		}

		if level.OrderCount == 0 {
			ob.bids.Delete(level.Price)
		}
	}

	if order.FilledQty > 0 {
		if order.RemainingQty == 0 {
			order.Status = OrderStatusFilled
		} else {
			order.Status = OrderStatusPartiallyFilled
		}
	}

	return trades
}

// matchMarketOrder 匹配市价单
func (ob *OrderBook) matchMarketOrder(order *Order) []*Trade {
	if order.Side == SideBuy {
		return ob.matchBuyOrder(order)
	}
	return ob.matchSellOrder(order)
}

// insertOrder 插入订单
func (ob *OrderBook) insertOrder(order *Order) {
	var book *PriceLevelMap
	if order.Side == SideBuy {
		book = ob.bids
	} else {
		book = ob.asks
	}

	level := book.GetOrCreate(order.Price)
	level.AddOrder(order)
	ob.orders.Store(order.OrderID, order)
}

// CancelOrder 取消订单
func (ob *OrderBook) CancelOrder(orderID string) (*Order, error) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	value, ok := ob.orders.Load(orderID)
	if !ok {
		return nil, fmt.Errorf("order not found: %s", orderID)
	}

	order := value.(*Order)

	var book *PriceLevelMap
	if order.Side == SideBuy {
		book = ob.bids
	} else {
		book = ob.asks
	}

	level, ok := book.Get(order.Price)
	if ok {
		level.RemoveOrder(orderID)
		if level.OrderCount == 0 {
			book.Delete(order.Price)
		}
	}

	ob.orders.Delete(orderID)
	order.Status = OrderStatusCancelled
	ob.version.Add(1)

	return order, nil
}

// GetOrder 获取订单
func (ob *OrderBook) GetOrder(orderID string) (*Order, bool) {
	value, ok := ob.orders.Load(orderID)
	if !ok {
		return nil, false
	}
	return value.(*Order), true
}

// GetBestBid 获取最优买价
func (ob *OrderBook) GetBestBid() (float64, int64) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	level, ok := ob.bids.First()
	if !ok {
		return 0, 0
	}
	return level.Price, level.TotalQty
}

// GetBestAsk 获取最优卖价
func (ob *OrderBook) GetBestAsk() (float64, int64) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	level, ok := ob.asks.First()
	if !ok {
		return 0, 0
	}
	return level.Price, level.TotalQty
}

// GetSpread 获取价差
func (ob *OrderBook) GetSpread() (float64, float64, float64) {
	bid, _ := ob.GetBestBid()
	ask, _ := ob.GetBestAsk()
	if bid == 0 || ask == 0 {
		return 0, 0, 0
	}
	spread := ask - bid
	midPrice := (bid + ask) / 2
	return spread, spread / midPrice * 100, midPrice
}

// GetDepth 获取深度
func (ob *OrderBook) GetDepth(levels int) ([]*PriceLevel, []*PriceLevel) {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bids := make([]*PriceLevel, 0, levels)
	asks := make([]*PriceLevel, 0, levels)

	count := 0
	ob.bids.Iterate(func(price float64, level *PriceLevel) bool {
		if count >= levels {
			return false
		}
		bids = append(bids, level)
		count++
		return true
	})

	count = 0
	ob.asks.Iterate(func(price float64, level *PriceLevel) bool {
		if count >= levels {
			return false
		}
		asks = append(asks, level)
		count++
		return true
	})

	return bids, asks
}

// Snapshot 订单簿快照
type Snapshot struct {
	Symbol    string               `json:"symbol"`
	Bids      []PriceLevelSnapshot `json:"bids"`
	Asks      []PriceLevelSnapshot `json:"asks"`
	Timestamp int64                `json:"timestamp"`
	UpdateID  int64                `json:"update_id"`
}

// PriceLevelSnapshot 价格档位快照
type PriceLevelSnapshot struct {
	Price    float64 `json:"price"`
	Quantity int64   `json:"quantity"`
	Count    int     `json:"count"`
}

// GetSnapshot 获取快照
func (ob *OrderBook) GetSnapshot(levels int) *Snapshot {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	bids, asks := ob.GetDepth(levels)

	snapshot := &Snapshot{
		Symbol:    ob.symbol,
		Timestamp: time.Now().UnixNano(),
		UpdateID:  ob.lastUpdateID,
	}

	for _, l := range bids {
		snapshot.Bids = append(snapshot.Bids, PriceLevelSnapshot{
			Price:    l.Price,
			Quantity: l.TotalQty,
			Count:    l.OrderCount,
		})
	}

	for _, l := range asks {
		snapshot.Asks = append(snapshot.Asks, PriceLevelSnapshot{
			Price:    l.Price,
			Quantity: l.TotalQty,
			Count:    l.OrderCount,
		})
	}

	return snapshot
}

// Stats 订单簿统计
type Stats struct {
	Symbol        string  `json:"symbol"`
	BidLevels     int     `json:"bid_levels"`
	AskLevels     int     `json:"ask_levels"`
	TotalBidQty   int64   `json:"total_bid_qty"`
	TotalAskQty   int64   `json:"total_ask_qty"`
	BestBid       float64 `json:"best_bid"`
	BestAsk       float64 `json:"best_ask"`
	Spread        float64 `json:"spread"`
	SpreadPercent float64 `json:"spread_percent"`
	MidPrice      float64 `json:"mid_price"`
	OrderCount    int     `json:"order_count"`
	Version       int64   `json:"version"`
}

// GetStats 获取统计信息
func (ob *OrderBook) GetStats() *Stats {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	stats := &Stats{
		Symbol:  ob.symbol,
		Version: ob.version.Load(),
	}

	stats.BidLevels = ob.bids.Len()
	stats.AskLevels = ob.asks.Len()

	ob.bids.Iterate(func(price float64, level *PriceLevel) bool {
		stats.TotalBidQty += level.TotalQty
		stats.OrderCount += level.OrderCount
		return true
	})

	ob.asks.Iterate(func(price float64, level *PriceLevel) bool {
		stats.TotalAskQty += level.TotalQty
		stats.OrderCount += level.OrderCount
		return true
	})

	if level, ok := ob.bids.First(); ok {
		stats.BestBid = level.Price
	}

	if level, ok := ob.asks.First(); ok {
		stats.BestAsk = level.Price
	}

	if stats.BestBid > 0 && stats.BestAsk > 0 {
		stats.Spread = stats.BestAsk - stats.BestBid
		stats.MidPrice = (stats.BestBid + stats.BestAsk) / 2
		if stats.MidPrice > 0 {
			stats.SpreadPercent = stats.Spread / stats.MidPrice * 100
		}
	}

	return stats
}

// createTrade 创建成交记录
func (ob *OrderBook) createTrade(buyOrder, sellOrder *Order, qty int64, price float64) *Trade {
	ob.tradeIDGen.Add(1)
	return &Trade{
		TradeID:     fmt.Sprintf("T%d", ob.tradeIDGen.Load()),
		Symbol:      ob.symbol,
		Price:       price,
		Quantity:    qty,
		BuyOrderID:  buyOrder.OrderID,
		SellOrderID: sellOrder.OrderID,
		Timestamp:   time.Now().UnixNano(),
	}
}

// Clear 清空订单簿
func (ob *OrderBook) Clear() {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	ob.bids = NewPriceLevelMap(true)
	ob.asks = NewPriceLevelMap(false)
	ob.orders = sync.Map{}
	ob.version.Add(1)
}

// 使用 structures 包的 SkipList（用于其他场景）
var _ = structures.SkipList[string, int]{}
