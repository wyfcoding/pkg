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
	orders map[string]*structures.RBNode // 订单索引映射。
	bids   *structures.RBTree            // 买单红黑树（价格从高到低）。
	asks   *structures.RBTree            // 卖单红黑树（价格从低到高）。
	mu     sync.RWMutex
}

// NewOrderBook 创建订单簿。
func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids:   structures.NewRBTree(true),  // 买单：价格高的优先。
		asks:   structures.NewRBTree(false), // 卖单：价格低的优先。
		orders: make(map[string]*structures.RBNode),
	}
}

// AddOrder 添加订单。
func (ob *OrderBook) AddOrder(order *types.Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	var node *structures.RBNode
	if order.Side == types.SideBuy {
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

	if node.Order.Side == types.SideBuy {
		ob.bids.DeleteNode(node)
	} else {
		ob.asks.DeleteNode(node)
	}
}

// GetBestBid 获取最优买价。
func (ob *OrderBook) GetBestBid() *types.Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.bids.GetBest()
}

// GetBestAsk 获取最优卖价。
func (ob *OrderBook) GetBestAsk() *types.Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.asks.GetBest()
}

// GetBids 获取买单列表。
func (ob *OrderBook) GetBids(depth int) []*types.OrderBookLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.getLevels(ob.bids, depth)
}

// GetAsks 获取卖单列表。
func (ob *OrderBook) GetAsks(depth int) []*types.OrderBookLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.getLevels(ob.asks, depth)
}

func (ob *OrderBook) getLevels(tree *structures.RBTree, depth int) []*types.OrderBookLevel {
	levels := make([]*types.OrderBookLevel, 0)
	if tree.Root == nil {
		return levels
	}

	it := tree.NewIterator()

	var currentLevel *types.OrderBookLevel

	for {
		order := it.Next()
		if order == nil {
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

var (
	orderPool = sync.Pool{
		New: func() any {
			return &types.Order{}
		},
	}
	tradePool = sync.Pool{
		New: func() any {
			return &types.Trade{}
		},
	}
)

// AcquireOrder 从对象池获取 Order。
func AcquireOrder() *types.Order {
	val, ok := orderPool.Get().(*types.Order)
	if !ok {
		return &types.Order{}
	}

	return val
}

// ReleaseOrder 将 Order 放回对象池。
func ReleaseOrder(o *types.Order) {
	*o = types.Order{} // 重置对象。
	orderPool.Put(o)
}

// AcquireTrade 从对象池获取 Trade。
func AcquireTrade() *types.Trade {
	val, ok := tradePool.Get().(*types.Trade)
	if !ok {
		return &types.Trade{}
	}

	return val
}

// ReleaseTrade 将 Trade 放回对象池。
func ReleaseTrade(t *types.Trade) {
	*t = types.Trade{} // 重置对象。
	tradePool.Put(t)
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

	var sideTree *structures.RBTree
	if order.Side == types.SideBuy {
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
	t := AcquireTrade()
	t.TradeID = generateTradeID()
	t.Symbol = taker.Symbol
	t.Quantity = qty
	t.Price = maker.Price
	t.Timestamp = time.Now().UnixNano()

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

		me.orderBook.RemoveOrder(o.OrderID)
		me.orderBook.AddOrder(o)
	} else {
		me.orderBook.RemoveOrder(o.OrderID)
	}
}

func (me *MatchingEngine) enqueueRemaining(o *types.Order) {
	if o.IsIceberg && o.Quantity.GreaterThan(o.DisplayQty) {
		o.HiddenQty = o.Quantity.Sub(o.DisplayQty)
		o.Quantity = o.DisplayQty
	}

	me.orderBook.AddOrder(o)
}

// GetTrades 获取所有交易记录。
func (me *MatchingEngine) GetTrades() []*types.Trade {
	me.mu.RLock()
	defer me.mu.RUnlock()

	result := make([]*types.Trade, len(me.trades))
	copy(result, me.trades)

	return result
}

// GetBids 获取买单列表。
func (me *MatchingEngine) GetBids(depth int) []*types.OrderBookLevel {
	me.mu.RLock()
	defer me.mu.RUnlock()

	return me.orderBook.GetBids(depth)
}

// GetAsks 获取卖单列表。
func (me *MatchingEngine) GetAsks(depth int) []*types.OrderBookLevel {
	me.mu.RLock()
	defer me.mu.RUnlock()

	return me.orderBook.GetAsks(depth)
}

func generateTradeID() string {
	id := idgen.GenID()
	return "TRD" + strconv.FormatUint(id, 10)
}
