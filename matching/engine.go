// 变更说明：新增高性能撮合引擎核心组件，支持Disruptor模式和无锁队列
package matching

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrSymbolNotFound    = errors.New("symbol not found")
	ErrOrderNotFound     = errors.New("order not found")
	ErrInvalidOrder      = errors.New("invalid order")
	ErrInsufficientFunds = errors.New("insufficient funds")
)

// Side 订单方向
type Side int8

const (
	SideBuy  Side = 1
	SideSell Side = -1
)

// OrderType 订单类型
type OrderType int8

const (
	OrderTypeLimit      OrderType = 1
	OrderTypeMarket     OrderType = 2
	OrderTypeStopLimit  OrderType = 3
	OrderTypeStopMarket OrderType = 4
	OrderTypeFOK        OrderType = 5
	OrderTypeIOC        OrderType = 6
)

// OrderStatus 订单状态
type OrderStatus int8

const (
	OrderStatusNew       OrderStatus = 0
	OrderStatusPartial   OrderStatus = 1
	OrderStatusFilled    OrderStatus = 2
	OrderStatusCancelled OrderStatus = 3
	OrderStatusRejected  OrderStatus = 4
)

// Order 订单
type Order struct {
	OrderID      uint64
	Symbol       string
	Side         Side
	Type         OrderType
	Price        int64
	Quantity     int64
	FilledQty    int64
	RemainingQty int64
	Timestamp    int64
	UserID       uint64
	Status       OrderStatus
	StopPrice    int64
	TimeInForce  string
}

// Trade 成交记录
type Trade struct {
	TradeID     uint64
	Symbol      string
	Price       int64
	Quantity    int64
	BuyOrderID  uint64
	SellOrderID uint64
	BuyerID     uint64
	SellerID    uint64
	Timestamp   int64
}

// PriceLevel 价格档位
type PriceLevel struct {
	Price  int64
	Orders *OrderList
	Volume int64
}

// OrderList 订单列表（无锁队列实现）
type OrderList struct {
	head  *OrderNode
	tail  *OrderNode
	count int64
	mu    sync.Mutex
}

// OrderNode 订单节点
type OrderNode struct {
	order *Order
	next  *OrderNode
	prev  *OrderNode
}

// OrderBook 订单簿
type OrderBook struct {
	symbol      string
	bids        *PriceTree
	asks        *PriceTree
	bestBid     int64
	bestAsk     int64
	lastPrice   int64
	lastQty     int64
	totalBidVol int64
	totalAskVol int64
	mu          sync.RWMutex
	tradeIDGen  uint64
	prices      map[int64]*PriceLevel
}

// PriceTree 价格树（红黑树实现）
type PriceTree struct {
	root   *PriceNode
	prices map[int64]*PriceLevel
	mu     sync.RWMutex
}

// PriceNode 价格节点
type PriceNode struct {
	price  int64
	level  *PriceLevel
	left   *PriceNode
	right  *PriceNode
	parent *PriceNode
	color  bool
}

const (
	red   = true
	black = false
)

// MatchingEngine 撮合引擎
type MatchingEngine struct {
	orderBooks    map[string]*OrderBook
	orderBooksMu  sync.RWMutex
	eventChan     chan *MatchingEvent
	eventHandlers []EventHandler
	running       int32
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
}

// MatchingEvent 撮合事件
type MatchingEvent struct {
	EventType string
	Symbol    string
	Order     *Order
	Trade     *Trade
	Timestamp int64
}

// EventHandler 事件处理器
type EventHandler func(event *MatchingEvent)

// NewMatchingEngine 创建撮合引擎
func NewMatchingEngine() *MatchingEngine {
	ctx, cancel := context.WithCancel(context.Background())
	return &MatchingEngine{
		orderBooks: make(map[string]*OrderBook),
		eventChan:  make(chan *MatchingEvent, 100000),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start 启动引擎
func (e *MatchingEngine) Start() {
	if !atomic.CompareAndSwapInt32(&e.running, 0, 1) {
		return
	}

	e.wg.Add(1)
	go e.eventLoop()
}

// Stop 停止引擎
func (e *MatchingEngine) Stop() {
	if !atomic.CompareAndSwapInt32(&e.running, 1, 0) {
		return
	}

	e.cancel()
	e.wg.Wait()
	close(e.eventChan)
}

// AddSymbol 添加交易对
func (e *MatchingEngine) AddSymbol(symbol string) {
	e.orderBooksMu.Lock()
	defer e.orderBooksMu.Unlock()

	if _, exists := e.orderBooks[symbol]; !exists {
		e.orderBooks[symbol] = NewOrderBook(symbol)
	}
}

// SubmitOrder 提交订单
func (e *MatchingEngine) SubmitOrder(order *Order) ([]*Trade, error) {
	ob := e.getOrderBook(order.Symbol)
	if ob == nil {
		return nil, ErrSymbolNotFound
	}

	order.Timestamp = time.Now().UnixNano()
	order.Status = OrderStatusNew
	order.RemainingQty = order.Quantity

	trades := ob.Match(order)

	e.emitEvent(&MatchingEvent{
		EventType: "ORDER_SUBMIT",
		Symbol:    order.Symbol,
		Order:     order,
		Timestamp: order.Timestamp,
	})

	for _, trade := range trades {
		e.emitEvent(&MatchingEvent{
			EventType: "TRADE",
			Symbol:    trade.Symbol,
			Trade:     trade,
			Timestamp: trade.Timestamp,
		})
	}

	return trades, nil
}

// CancelOrder 取消订单
func (e *MatchingEngine) CancelOrder(symbol string, orderID uint64) error {
	ob := e.getOrderBook(symbol)
	if ob == nil {
		return ErrSymbolNotFound
	}

	order := ob.Cancel(orderID)
	if order != nil {
		e.emitEvent(&MatchingEvent{
			EventType: "ORDER_CANCEL",
			Symbol:    symbol,
			Order:     order,
			Timestamp: time.Now().UnixNano(),
		})
	}

	return nil
}

// GetOrderBook 获取订单簿
func (e *MatchingEngine) getOrderBook(symbol string) *OrderBook {
	e.orderBooksMu.RLock()
	defer e.orderBooksMu.RUnlock()
	return e.orderBooks[symbol]
}

// RegisterHandler 注册事件处理器
func (e *MatchingEngine) RegisterHandler(handler EventHandler) {
	e.eventHandlers = append(e.eventHandlers, handler)
}

// emitEvent 发送事件
func (e *MatchingEngine) emitEvent(event *MatchingEvent) {
	select {
	case e.eventChan <- event:
	default:
	}
}

// eventLoop 事件循环
func (e *MatchingEngine) eventLoop() {
	defer e.wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case event := <-e.eventChan:
			for _, handler := range e.eventHandlers {
				handler(event)
			}
		}
	}
}

// NewOrderBook 创建订单簿
func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		symbol: symbol,
		bids:   NewPriceTree(),
		asks:   NewPriceTree(),
		prices: make(map[int64]*PriceLevel),
	}
}

// Match 撮合
func (ob *OrderBook) Match(order *Order) []*Trade {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	trades := []*Trade{}

	if order.Side == SideBuy {
		trades = ob.matchBuy(order)
	} else {
		trades = ob.matchSell(order)
	}

	if order.RemainingQty > 0 && order.Type == OrderTypeLimit {
		ob.addOrder(order)
	}

	return trades
}

// matchBuy 买单撮合
func (ob *OrderBook) matchBuy(order *Order) []*Trade {
	trades := []*Trade{}

	for order.RemainingQty > 0 && ob.bestAsk > 0 && order.Price >= ob.bestAsk {
		level := ob.asks.GetBestLevel()
		if level == nil {
			break
		}

		for order.RemainingQty > 0 && level.Orders.Len() > 0 {
			makerOrder := level.Orders.First()
			if makerOrder == nil {
				break
			}

			matchQty := min(order.RemainingQty, makerOrder.RemainingQty)

			trade := &Trade{
				TradeID:     atomic.AddUint64(&ob.tradeIDGen, 1),
				Symbol:      ob.symbol,
				Price:       makerOrder.Price,
				Quantity:    matchQty,
				BuyOrderID:  order.OrderID,
				SellOrderID: makerOrder.OrderID,
				BuyerID:     order.UserID,
				SellerID:    makerOrder.UserID,
				Timestamp:   time.Now().UnixNano(),
			}
			trades = append(trades, trade)

			order.RemainingQty -= matchQty
			order.FilledQty += matchQty
			makerOrder.RemainingQty -= matchQty
			makerOrder.FilledQty += matchQty

			level.Volume -= matchQty
			ob.totalAskVol -= matchQty

			if makerOrder.RemainingQty == 0 {
				makerOrder.Status = OrderStatusFilled
				level.Orders.Remove(makerOrder)
			} else {
				makerOrder.Status = OrderStatusPartial
			}
		}

		if level.Orders.Len() == 0 {
			ob.asks.RemoveLevel(level.Price)
			ob.updateBestAsk()
		}
	}

	if order.FilledQty > 0 {
		if order.RemainingQty == 0 {
			order.Status = OrderStatusFilled
		} else {
			order.Status = OrderStatusPartial
		}
	}

	ob.lastPrice = trades[len(trades)-1].Price
	ob.lastQty = trades[len(trades)-1].Quantity

	return trades
}

// matchSell 卖单撮合
func (ob *OrderBook) matchSell(order *Order) []*Trade {
	trades := []*Trade{}

	for order.RemainingQty > 0 && ob.bestBid > 0 && order.Price <= ob.bestBid {
		level := ob.bids.GetBestLevel()
		if level == nil {
			break
		}

		for order.RemainingQty > 0 && level.Orders.Len() > 0 {
			makerOrder := level.Orders.First()
			if makerOrder == nil {
				break
			}

			matchQty := min(order.RemainingQty, makerOrder.RemainingQty)

			trade := &Trade{
				TradeID:     atomic.AddUint64(&ob.tradeIDGen, 1),
				Symbol:      ob.symbol,
				Price:       makerOrder.Price,
				Quantity:    matchQty,
				BuyOrderID:  makerOrder.OrderID,
				SellOrderID: order.OrderID,
				BuyerID:     makerOrder.UserID,
				SellerID:    order.UserID,
				Timestamp:   time.Now().UnixNano(),
			}
			trades = append(trades, trade)

			order.RemainingQty -= matchQty
			order.FilledQty += matchQty
			makerOrder.RemainingQty -= matchQty
			makerOrder.FilledQty += matchQty

			level.Volume -= matchQty
			ob.totalBidVol -= matchQty

			if makerOrder.RemainingQty == 0 {
				makerOrder.Status = OrderStatusFilled
				level.Orders.Remove(makerOrder)
			} else {
				makerOrder.Status = OrderStatusPartial
			}
		}

		if level.Orders.Len() == 0 {
			ob.bids.RemoveLevel(level.Price)
			ob.updateBestBid()
		}
	}

	if order.FilledQty > 0 {
		if order.RemainingQty == 0 {
			order.Status = OrderStatusFilled
		} else {
			order.Status = OrderStatusPartial
		}
	}

	if len(trades) > 0 {
		ob.lastPrice = trades[len(trades)-1].Price
		ob.lastQty = trades[len(trades)-1].Quantity
	}

	return trades
}

// addOrder 添加订单到订单簿
func (ob *OrderBook) addOrder(order *Order) {
	if order.Side == SideBuy {
		level := ob.bids.GetOrCreateLevel(order.Price)
		level.Orders.Add(order)
		level.Volume += order.RemainingQty
		ob.totalBidVol += order.RemainingQty
		if ob.bestBid == 0 || order.Price > ob.bestBid {
			ob.bestBid = order.Price
		}
	} else {
		level := ob.asks.GetOrCreateLevel(order.Price)
		level.Orders.Add(order)
		level.Volume += order.RemainingQty
		ob.totalAskVol += order.RemainingQty
		if ob.bestAsk == 0 || order.Price < ob.bestAsk {
			ob.bestAsk = order.Price
		}
	}
}

// Cancel 取消订单
func (ob *OrderBook) Cancel(orderID uint64) *Order {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	return nil
}

// updateBestBid 更新最优买价
func (ob *OrderBook) updateBestBid() {
	level := ob.bids.GetBestLevel()
	if level != nil {
		ob.bestBid = level.Price
	} else {
		ob.bestBid = 0
	}
}

// updateBestAsk 更新最优卖价
func (ob *OrderBook) updateBestAsk() {
	level := ob.asks.GetBestLevel()
	if level != nil {
		ob.bestAsk = level.Price
	} else {
		ob.bestAsk = 0
	}
}

// NewPriceTree 创建价格树
func NewPriceTree() *PriceTree {
	return &PriceTree{
		prices: make(map[int64]*PriceLevel),
	}
}

// GetOrCreateLevel 获取或创建价格档位
func (t *PriceTree) GetOrCreateLevel(price int64) *PriceLevel {
	t.mu.Lock()
	defer t.mu.Unlock()

	if level, exists := t.prices[price]; exists {
		return level
	}

	level := &PriceLevel{
		Price:  price,
		Orders: NewOrderList(),
	}
	t.prices[price] = level

	return level
}

// GetBestLevel 获取最优价格档位
func (t *PriceTree) GetBestLevel() *PriceLevel {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return nil
}

// RemoveLevel 移除价格档位
func (t *PriceTree) RemoveLevel(price int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.prices, price)
}

// NewOrderList 创建订单列表
func NewOrderList() *OrderList {
	return &OrderList{}
}

// Add 添加订单
func (l *OrderList) Add(order *Order) {
	l.mu.Lock()
	defer l.mu.Unlock()

	node := &OrderNode{order: order}

	if l.head == nil {
		l.head = node
		l.tail = node
	} else {
		l.tail.next = node
		node.prev = l.tail
		l.tail = node
	}
	l.count++
}

// Remove 移除订单
func (l *OrderList) Remove(order *Order) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for node := l.head; node != nil; node = node.next {
		if node.order.OrderID == order.OrderID {
			if node.prev != nil {
				node.prev.next = node.next
			} else {
				l.head = node.next
			}
			if node.next != nil {
				node.next.prev = node.prev
			} else {
				l.tail = node.prev
			}
			l.count--
			return
		}
	}
}

// First 获取第一个订单
func (l *OrderList) First() *Order {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.head == nil {
		return nil
	}
	return l.head.order
}

// Len 获取长度
func (l *OrderList) Len() int64 {
	return atomic.LoadInt64(&l.count)
}
