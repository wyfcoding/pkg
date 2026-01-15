package finance

import (
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wyfcoding/pkg/algorithm/structures"
	"github.com/wyfcoding/pkg/algorithm/types"
	"github.com/wyfcoding/pkg/idgen"
)

// OrderBook 订单簿核心结构，管理买卖双边挂单。
type OrderBook struct {
	orders map[string]*structures.RBNode[*types.Order] // 订单索引映射。
	bids   *structures.RBTree[*types.Order]            // 买单红黑树（价格从高到低）。
	asks   *structures.RBTree[*types.Order]            // 卖单红黑树（价格从低到高）。
	mu     sync.RWMutex
}

// NewOrderBook 创建订单簿。
func NewOrderBook() *OrderBook {
	// 买单：价格高的优先。
	bidCompare := func(a, b *types.Order) int {
		priceCmp := b.Price.Cmp(a.Price) // 反向比较实现 MaxTree
		if priceCmp == 0 {
			if a.Timestamp < b.Timestamp {
				return -1
			}
			if a.Timestamp > b.Timestamp {
				return 1
			}
			return 0
		}
		return priceCmp
	}

	// 卖单：价格低的优先。
	askCompare := func(a, b *types.Order) int {
		priceCmp := a.Price.Cmp(b.Price)
		if priceCmp == 0 {
			if a.Timestamp < b.Timestamp {
				return -1
			}
			if a.Timestamp > b.Timestamp {
				return 1
			}
			return 0
		}
		return priceCmp
	}

	return &OrderBook{
		bids:   structures.NewRBTree(bidCompare),
		asks:   structures.NewRBTree(askCompare),
		orders: make(map[string]*structures.RBNode[*types.Order]),
	}
}

// AddOrder 添加订单（线程安全）。
func (ob *OrderBook) AddOrder(order *types.Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	ob.addOrder(order)
}

// addOrder 添加订单（内部无锁）。
func (ob *OrderBook) addOrder(order *types.Order) {
	var node *structures.RBNode[*types.Order]
	if order.Side == types.SideBuy {
		node = ob.bids.Insert(order)
	} else {
		node = ob.asks.Insert(order)
	}

	ob.orders[order.OrderID] = node
}

// RemoveOrder 移除订单（线程安全）。
func (ob *OrderBook) RemoveOrder(orderID string) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	ob.removeOrder(orderID)
}

// removeOrder 移除订单（内部无锁）。
func (ob *OrderBook) removeOrder(orderID string) {
	node, ok := ob.orders[orderID]
	if !ok {
		return
	}

	delete(ob.orders, orderID)

	if node.Value.Side == types.SideBuy {
		ob.bids.DeleteNode(node)
	} else {
		ob.asks.DeleteNode(node)
	}
}

// GetBestBid 获取最优买价（线程安全）。
func (ob *OrderBook) GetBestBid() *types.Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	node := ob.bids.Min()
	if node == nil {
		return nil
	}
	return node.Value
}

// GetBestAsk 获取最优卖价（线程安全）。
func (ob *OrderBook) GetBestAsk() *types.Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	node := ob.asks.Min()
	if node == nil {
		return nil
	}
	return node.Value
}

// GetBids 获取买单列表（线程安全）。
func (ob *OrderBook) GetBids(depth int) []*types.OrderBookLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.getLevels(ob.bids, depth)
}

// GetAsks 获取卖单列表（线程安全）。
func (ob *OrderBook) GetAsks(depth int) []*types.OrderBookLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.getLevels(ob.asks, depth)
}

func (ob *OrderBook) getLevels(tree *structures.RBTree[*types.Order], depth int) []*types.OrderBookLevel {
	levels := make([]*types.OrderBookLevel, 0)
	it := tree.NewIterator()

	var currentLevel *types.OrderBookLevel

	for {
		order, ok := it.Next()
		if !ok {
			break
		}

		if currentLevel == nil || !currentLevel.Price.Equal(order.Price) {
			if len(levels) >= depth {
				break
			}

			currentLevel = &types.OrderBookLevel{
				Price:  order.Price,
				Qty:    decimal.Zero,
				Orders: make([]*types.Order, 0),
			}
			levels = append(levels, currentLevel)
		}

		currentLevel.Qty = currentLevel.Qty.Add(order.Quantity)
		currentLevel.Orders = append(currentLevel.Orders, order)
	}

	return levels
}

// MatchingEngine 撮合引擎。
type MatchingEngine struct {
	orderBook *OrderBook
	trades    []*types.Trade
	mu        sync.RWMutex
}

// NewMatchingEngine 创建撮合引擎。
func NewMatchingEngine() *MatchingEngine {
	return &MatchingEngine{
		orderBook: NewOrderBook(),
		trades:    make([]*types.Trade, 0),
	}
}

// Match 撮合订单。
func (me *MatchingEngine) Match(order *types.Order) []*types.Trade {
	me.mu.Lock()
	defer me.mu.Unlock()

	trades := make([]*types.Trade, 0)

	if order.PostOnly && me.wouldMatch(order) {
		return trades
	}

	var sideTree *structures.RBTree[*types.Order]
	if order.Side == types.SideBuy {
		sideTree = me.orderBook.asks
	} else {
		sideTree = me.orderBook.bids
	}

	for order.Quantity.GreaterThan(decimal.Zero) {
		bestCounterNode := sideTree.Min()
		if bestCounterNode == nil {
			break
		}
		bestCounter := bestCounterNode.Value
		if !me.canMatch(order, bestCounter) {
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

func (me *MatchingEngine) wouldMatch(order *types.Order) bool {
	if order.Side == types.SideBuy {
		bestAsk := me.orderBook.GetBestAsk()
		return bestAsk != nil && bestAsk.Price.LessThanOrEqual(order.Price)
	}

	bestBid := me.orderBook.GetBestBid()
	return bestBid != nil && bestBid.Price.GreaterThanOrEqual(order.Price)
}

func (me *MatchingEngine) canMatch(order, counter *types.Order) bool {
	if order.Side == types.SideBuy {
		return counter.Price.LessThanOrEqual(order.Price)
	}

	return counter.Price.GreaterThanOrEqual(order.Price)
}

func (me *MatchingEngine) executeTrade(taker, maker *types.Order, qty decimal.Decimal) *types.Trade {
	t := &types.Trade{
		TradeID:   generateTradeID(),
		Symbol:    taker.Symbol,
		Quantity:  qty,
		Price:     maker.Price,
		Timestamp: time.Now().UnixNano(),
	}

	if taker.Side == types.SideBuy {
		t.BuyOrderID, t.BuyUserID = taker.OrderID, taker.UserID
		t.SellOrderID, t.SellUserID = maker.OrderID, maker.UserID
	} else {
		t.BuyOrderID, t.BuyUserID = maker.OrderID, maker.UserID
		t.SellOrderID, t.SellUserID = taker.OrderID, taker.UserID
	}

	return t
}

func (me *MatchingEngine) handleFinishedOrder(o *types.Order) {
	if o.IsIceberg && o.HiddenQty.GreaterThan(decimal.Zero) {
		refreshQty := decimal.Min(o.DisplayQty, o.HiddenQty)
		o.Quantity = refreshQty
		o.HiddenQty = o.HiddenQty.Sub(refreshQty)
		o.Timestamp = time.Now().UnixNano()

		me.orderBook.removeOrder(o.OrderID)
		me.orderBook.addOrder(o)
	} else {
		me.orderBook.removeOrder(o.OrderID)
	}
}

func (me *MatchingEngine) enqueueRemaining(o *types.Order) {
	if o.IsIceberg && o.Quantity.GreaterThan(o.DisplayQty) {
		o.HiddenQty = o.Quantity.Sub(o.DisplayQty)
		o.Quantity = o.DisplayQty
	}

	me.orderBook.addOrder(o)
}

// GetTrades 获取所有交易记录。
func (me *MatchingEngine) GetTrades() []*types.Trade {
	me.mu.RLock()
	defer me.mu.RUnlock()

	result := make([]*types.Trade, len(me.trades))
	copy(result, me.trades)

	return result
}

func generateTradeID() string {
	id := idgen.GenID()
	return "TRD" + strconv.FormatUint(id, 10)
}
