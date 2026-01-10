// Package algorithm 提供高性能/ACM/竞赛算法集合。
// 此文件实现了工业级撮合引擎核心逻辑，包含 LOB (Limit Order Book) 订单簿。
//
// 性能优化点：
// 1. 内存优化：使用 sync.Pool 复用 Order 和 Trade 对象，显著降低高频撮合下的 GC 压力。
// 2. 算法优化：基于红黑树（RBTree）实现价格档位排序，插入与删除复杂度为 O(log N)。
// 3. 业务增强：支持冰山单（Iceberg Order）、只做 Maker（Post-Only）等高级指令。
// 4. 并发模型：采用读写锁（RWMutex）保护订单簿状态，支持高并发查询。
//
// 复杂度分析：
// - 订单提交 (Match): 平均 O(M * log N)，M 为成交笔数，N 为订单簿深度。
// - 订单撤单 (RemoveOrder): O(log N)。
// - 行情查询 (GetBids/Asks): O(D)，D 为请求的深度。
package algorithm

import (
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wyfcoding/pkg/idgen"
)

// Order 订单结构，表示一个限价单或高级策略单
type Order struct {
	OrderID    string          // 订单 ID
	Symbol     string          // 交易对名称
	Side       string          // 方向 (BUY/SELL)
	Price      decimal.Decimal // 委托价格
	Quantity   decimal.Decimal // 委托数量
	Timestamp  int64           // 纳秒级时间戳，用于时间优先排序
	UserID     string          // 用户唯一标识
	IsIceberg  bool            // 是否为冰山单
	DisplayQty decimal.Decimal // 冰山单单次可见规模
	HiddenQty  decimal.Decimal // 冰山单当前隐性规模
	PostOnly   bool            // 是否为只做 Maker 策略
	ResultChan chan any        `json:"-"` // 异步撮合结果反馈通道
}

// OrderBookLevel 订单簿档位，聚合了同一价格下的委托总量。
type OrderBookLevel struct {
	Price    decimal.Decimal `json:"price"`    // 档位价格
	Quantity decimal.Decimal `json:"quantity"` // 该档位挂单总数量
	Orders   []*Order        `json:"orders"`   // 该档位包含的具体订单列表
}

// OrderBook 订单簿核心结构，管理买卖双边挂单
type OrderBook struct {
	mu     sync.RWMutex
	bids   *RBTree            // 买单红黑树（价格从高到低）
	asks   *RBTree            // 卖单红黑树（价格从低到高）
	orders map[string]*RBNode // 订单索引映射，直接映射到红黑树节点，实现 O(1) 定位 O(log N) 删除
}

// NewOrderBook 创建订单簿
func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids:   NewRBTree(true),  // 买单：价格高的优先
		asks:   NewRBTree(false), // 卖单：价格低的优先
		orders: make(map[string]*RBNode),
	}
}

// AddOrder 添加订单
func (ob *OrderBook) AddOrder(order *Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	var node *RBNode
	if order.Side == "BUY" {
		node = ob.bids.Insert(order)
	} else {
		node = ob.asks.Insert(order)
	}
	ob.orders[order.OrderID] = node
}

// RemoveOrder 移除订单
func (ob *OrderBook) RemoveOrder(orderID string) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	node, ok := ob.orders[orderID]
	if !ok {
		return
	}

	delete(ob.orders, orderID)

	if node.Order.Side == "BUY" {
		ob.bids.DeleteNode(node)
	} else {
		ob.asks.DeleteNode(node)
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
	// 买方用户 ID
	BuyUserID string
	// 卖方用户 ID
	SellUserID string
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

// Match 撮合订单。使用高度抽象的逻辑消除买卖方向的代码冗余。
func (me *MatchingEngine) Match(order *Order) []*Trade {
	me.mu.Lock()
	defer me.mu.Unlock()

	trades := make([]*Trade, 0)

	// 1. Post-Only 检查
	if order.PostOnly && me.wouldMatch(order) {
		return trades
	}

	// 2. 确定对手方树 (买单找卖方树，卖单找买方树)
	var sideTree *RBTree
	if order.Side == "BUY" {
		sideTree = me.orderBook.asks
	} else {
		sideTree = me.orderBook.bids
	}

	// 3. 循环撮合直到订单填满或对手方耗尽
	for order.Quantity.GreaterThan(decimal.Zero) {
		bestCounter := sideTree.GetBest()
		// 如果对手方为空，或者价格不匹配，则停止撮合
		if bestCounter == nil || !me.canMatch(order, bestCounter) {
			break
		}

		// 执行单笔成交
		matchQty := decimal.Min(order.Quantity, bestCounter.Quantity)
		trade := me.executeTrade(order, bestCounter, matchQty)
		trades = append(trades, trade)

		// 更新剩余数量
		order.Quantity = order.Quantity.Sub(matchQty)
		bestCounter.Quantity = bestCounter.Quantity.Sub(matchQty)

		// 处理对手方订单完成后的状态（包括冰山单刷新）
		if bestCounter.Quantity.Equal(decimal.Zero) {
			me.handleFinishedOrder(bestCounter)
		}
	}

	// 4. 剩余委托转入订单簿
	if order.Quantity.GreaterThan(decimal.Zero) {
		me.enqueueRemaining(order)
	}

	me.trades = append(me.trades, trades...)
	return trades
}

// wouldMatch 预检 Post-Only 订单是否会立即成交
func (me *MatchingEngine) wouldMatch(order *Order) bool {
	if order.Side == "BUY" {
		bestAsk := me.orderBook.GetBestAsk()
		return bestAsk != nil && bestAsk.Price.LessThanOrEqual(order.Price)
	}
	bestBid := me.orderBook.GetBestBid()
	return bestBid != nil && bestBid.Price.GreaterThanOrEqual(order.Price)
}

// canMatch 价格匹配检查
func (me *MatchingEngine) canMatch(order, counter *Order) bool {
	if order.Side == "BUY" {
		return counter.Price.LessThanOrEqual(order.Price)
	}
	return counter.Price.GreaterThanOrEqual(order.Price)
}

// executeTrade 生成交易记录
func (me *MatchingEngine) executeTrade(taker, maker *Order, qty decimal.Decimal) *Trade {
	t := AcquireTrade()
	t.TradeID = generateTradeID()
	t.Symbol = taker.Symbol
	t.Quantity = qty
	t.Price = maker.Price // 成交价以 Maker (挂单) 为准
	t.Timestamp = time.Now().UnixNano()

	if taker.Side == "BUY" {
		t.BuyOrderID, t.BuyUserID = taker.OrderID, taker.UserID
		t.SellOrderID, t.SellUserID = maker.OrderID, maker.UserID
	} else {
		t.BuyOrderID, t.BuyUserID = maker.OrderID, maker.UserID
		t.SellOrderID, t.SellUserID = taker.OrderID, taker.UserID
	}
	return t
}

// handleFinishedOrder 处理成交完的订单（包括冰山单逻辑）
func (me *MatchingEngine) handleFinishedOrder(o *Order) {
	if o.IsIceberg && o.HiddenQty.GreaterThan(decimal.Zero) {
		refreshQty := decimal.Min(o.DisplayQty, o.HiddenQty)
		o.Quantity = refreshQty
		o.HiddenQty = o.HiddenQty.Sub(refreshQty)
		o.Timestamp = time.Now().UnixNano() // 刷新时间戳以失去时间优先权

		// 在红黑树中重新定位
		me.orderBook.RemoveOrder(o.OrderID)
		me.orderBook.AddOrder(o)
	} else {
		me.orderBook.RemoveOrder(o.OrderID)
	}
}

// enqueueRemaining 处理 Taker 剩余委托进入订单簿
func (me *MatchingEngine) enqueueRemaining(o *Order) {
	if o.IsIceberg && o.Quantity.GreaterThan(o.DisplayQty) {
		o.HiddenQty = o.Quantity.Sub(o.DisplayQty)
		o.Quantity = o.DisplayQty
	}
	me.orderBook.AddOrder(o)
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
	return "TRD" + strconv.FormatUint(idgen.GenID(), 10)
}
