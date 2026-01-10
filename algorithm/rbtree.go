// Package algorithm 提供红黑树数据结构和订单簿实现。
package algorithm

import "sync"

// Color 红黑树节点颜色。
type Color bool

const (
	// ColorRed 红色节点。
	ColorRed Color = true
	// ColorBlack 黑色节点。
	ColorBlack Color = false
)

// RBNode 红黑树节点。
type RBNode struct {
	Order  *Order
	Left   *RBNode
	Right  *RBNode
	Parent *RBNode
	Color  Color
}

// RBTree 红黑树数据结构实现。
// 在撮合引擎中，红黑树用于维护有序订单簿（OrderBook）。
// 红黑树能够保证插入、删除和查找的时间复杂度均为 O(log N)，满足高频交易的需求。
type RBTree struct {
	Root *RBNode
	// IsMaxTree true 表示按价格降序排列（买单树，高价优先）。
	// false 表示按价格升序排列（卖单树，低价优先）。
	IsMaxTree bool
	Size      int // 树中节点的总数。
}

var rbNodePool = sync.Pool{
	New: func() any {
		return &RBNode{}
	},
}

func newRBNode(order *Order, color Color) *RBNode {
	node, ok := rbNodePool.Get().(*RBNode)
	if !ok {
		return &RBNode{
			Order: order,
			Color: color,
		}
	}

	node.Order = order
	node.Color = color
	node.Left = nil
	node.Right = nil
	node.Parent = nil

	return node
}

func releaseRBNode(node *RBNode) {
	node.Order = nil
	node.Left = nil
	node.Right = nil
	node.Parent = nil
	rbNodePool.Put(node)
}

// NewRBTree 创建并返回一个指定排序方向的红黑树。
func NewRBTree(isMaxTree bool) *RBTree {
	return &RBTree{
		IsMaxTree: isMaxTree,
		Root:      nil,
		Size:      0,
	}
}

// Less 决定节点 a 是否应该排在节点 b 的左侧。
func (t *RBTree) Less(nodeA, nodeB *Order) bool {
	priceCmp := nodeA.Price.Cmp(nodeB.Price)
	if priceCmp == 0 {
		// 价格相同，早到的订单排在左边（中序遍历先访问）。
		return nodeA.Timestamp < nodeB.Timestamp
	}

	if t.IsMaxTree {
		// 买单树：价格高的订单优先级更高，排在左边。
		return priceCmp > 0
	}

	// 卖单树：价格低的订单优先级更高，排在左边。
	return priceCmp < 0
}

// Insert 向红黑树中插入一个新的订单。
func (t *RBTree) Insert(order *Order) *RBNode {
	newNode := newRBNode(order, ColorRed)
	var parentNode *RBNode

	currNode := t.Root

	for currNode != nil {
		parentNode = currNode
		if t.Less(newNode.Order, currNode.Order) {
			currNode = currNode.Left
		} else {
			currNode = currNode.Right
		}
	}

	newNode.Parent = parentNode

	if parentNode == nil {
		t.Root = newNode
	} else if t.Less(newNode.Order, parentNode.Order) {
		parentNode.Left = newNode
	} else {
		parentNode.Right = newNode
	}

	t.insertFixup(newNode)

	t.Size++

	return newNode
}

func (t *RBTree) insertFixup(targetNode *RBNode) {
	node := targetNode

	for node.Parent != nil && node.Parent.Color == ColorRed {
		if node.Parent == node.Parent.Parent.Left {
			uncle := node.Parent.Parent.Right
			if uncle != nil && uncle.Color == ColorRed {
				node.Parent.Color = ColorBlack
				uncle.Color = ColorBlack
				node.Parent.Parent.Color = ColorRed
				node = node.Parent.Parent
			} else {
				if node == node.Parent.Right {
					node = node.Parent
					t.leftRotate(node)
				}

				node.Parent.Color = ColorBlack
				node.Parent.Parent.Color = ColorRed

				t.rightRotate(node.Parent.Parent)
			}
		} else {
			uncle := node.Parent.Parent.Left
			if uncle != nil && uncle.Color == ColorRed {
				node.Parent.Color = ColorBlack
				uncle.Color = ColorBlack
				node.Parent.Parent.Color = ColorRed
				node = node.Parent.Parent
			} else {
				if node == node.Parent.Left {
					node = node.Parent
					t.rightRotate(node)
				}

				node.Parent.Color = ColorBlack
				node.Parent.Parent.Color = ColorRed

				t.leftRotate(node.Parent.Parent)
			}
		}
	}

	t.Root.Color = ColorBlack
}

func (t *RBTree) leftRotate(nodeX *RBNode) {
	nodeY := nodeX.Right
	nodeX.Right = nodeY.Left

	if nodeY.Left != nil {
		nodeY.Left.Parent = nodeX
	}

	nodeY.Parent = nodeX.Parent

	if nodeX.Parent == nil {
		t.Root = nodeY
	} else if nodeX == nodeX.Parent.Left {
		nodeX.Parent.Left = nodeY
	} else {
		nodeX.Parent.Right = nodeY
	}

	nodeY.Left = nodeX
	nodeX.Parent = nodeY
}

func (t *RBTree) rightRotate(nodeY *RBNode) {
	nodeX := nodeY.Left
	nodeY.Left = nodeX.Right

	if nodeX.Right != nil {
		nodeX.Right.Parent = nodeY
	}

	nodeX.Parent = nodeY.Parent

	if nodeY.Parent == nil {
		t.Root = nodeX
	} else if nodeY == nodeY.Parent.Right {
		nodeY.Parent.Right = nodeX
	} else {
		nodeY.Parent.Left = nodeX
	}

	nodeX.Right = nodeY
	nodeY.Parent = nodeX
}

// Delete 根据 Order 寻找并删除节点。
func (t *RBTree) Delete(order *Order) {
	target := t.find(order)
	if target == nil {
		return
	}

	t.DeleteNode(target)
}

func (t *RBTree) find(order *Order) *RBNode {
	curr := t.Root
	for curr != nil {
		if curr.Order.OrderID == order.OrderID {
			return curr
		}

		if t.Less(order, curr.Order) {
			curr = curr.Left
		} else {
			curr = curr.Right
		}
	}

	return nil
}

// DeleteNode 删除指定节点。
func (t *RBTree) DeleteNode(target *RBNode) {
	var replaceNode, childNode *RBNode

	if target.Left == nil || target.Right == nil {
		replaceNode = target
	} else {
		replaceNode = t.successor(target)
	}

	if replaceNode.Left != nil {
		childNode = replaceNode.Left
	} else {
		childNode = replaceNode.Right
	}

	if childNode != nil {
		childNode.Parent = replaceNode.Parent
	}

	if replaceNode.Parent == nil {
		t.Root = childNode
	} else if replaceNode == replaceNode.Parent.Left {
		replaceNode.Parent.Left = childNode
	} else {
		replaceNode.Parent.Right = childNode
	}

	if replaceNode != target {
		target.Order = replaceNode.Order
	}

	if replaceNode.Color == ColorBlack && childNode != nil {
		t.deleteFixup(childNode)
	}

	releaseRBNode(replaceNode)

	t.Size--
}

func (t *RBTree) successor(node *RBNode) *RBNode {
	if node.Right != nil {
		return t.minimum(node.Right)
	}

	curr := node
	parent := node.Parent

	for parent != nil && curr == parent.Right {
		curr = parent
		parent = parent.Parent
	}

	return parent
}

func (t *RBTree) minimum(node *RBNode) *RBNode {
	curr := node
	for curr.Left != nil {
		curr = curr.Left
	}

	return curr
}

func (t *RBTree) deleteFixup(targetNode *RBNode) {
	node := targetNode

	for node != t.Root && node.Color == ColorBlack {
		if node == node.Parent.Left {
			node = t.fixDeleteLeft(node)
		} else {
			node = t.fixDeleteRight(node)
		}
	}

	node.Color = ColorBlack
}

func (t *RBTree) fixDeleteLeft(node *RBNode) *RBNode {
	sibling := node.Parent.Right
	if sibling.Color == ColorRed {
		sibling.Color = ColorBlack
		node.Parent.Color = ColorRed

		t.leftRotate(node.Parent)

		sibling = node.Parent.Right
	}

	if (sibling.Left == nil || sibling.Left.Color == ColorBlack) &&
		(sibling.Right == nil || sibling.Right.Color == ColorBlack) {
		sibling.Color = ColorRed

		return node.Parent
	}

	if sibling.Right == nil || sibling.Right.Color == ColorBlack {
		if sibling.Left != nil {
			sibling.Left.Color = ColorBlack
		}

		sibling.Color = ColorRed

		t.rightRotate(sibling)

		sibling = node.Parent.Right
	}

	sibling.Color = node.Parent.Color
	node.Parent.Color = ColorBlack

	if sibling.Right != nil {
		sibling.Right.Color = ColorBlack
	}

	t.leftRotate(node.Parent)

	return t.Root
}

func (t *RBTree) fixDeleteRight(node *RBNode) *RBNode {
	sibling := node.Parent.Left
	if sibling.Color == ColorRed {
		sibling.Color = ColorBlack
		node.Parent.Color = ColorRed

		t.rightRotate(node.Parent)

		sibling = node.Parent.Left
	}

	if (sibling.Right == nil || sibling.Right.Color == ColorBlack) &&
		(sibling.Left == nil || sibling.Left.Color == ColorBlack) {
		sibling.Color = ColorRed

		return node.Parent
	}

	if sibling.Left == nil || sibling.Left.Color == ColorBlack {
		if sibling.Right != nil {
			sibling.Right.Color = ColorBlack
		}

		sibling.Color = ColorRed

		t.leftRotate(sibling)

		sibling = node.Parent.Left
	}

	sibling.Color = node.Parent.Color
	node.Parent.Color = ColorBlack

	if sibling.Left != nil {
		sibling.Left.Color = ColorBlack
	}

	t.rightRotate(node.Parent)

	return t.Root
}

// GetBest 获取最优订单。
func (t *RBTree) GetBest() *Order {
	if t.Root == nil {
		return nil
	}

	return t.minimum(t.Root).Order
}

// Iterator 迭代器。
type Iterator struct {
	stack []*RBNode
}

// NewIterator 创建迭代器。
func (t *RBTree) NewIterator() *Iterator {
	iterator := &Iterator{stack: make([]*RBNode, 0)}
	iterator.pushLeft(t.Root)

	return iterator
}

func (it *Iterator) pushLeft(node *RBNode) {
	curr := node
	for curr != nil {
		it.stack = append(it.stack, curr)
		curr = curr.Left
	}
}

// Next 返回下一个订单。
func (it *Iterator) Next() *Order {
	if len(it.stack) == 0 {
		return nil
	}

	node := it.stack[len(it.stack)-1]
	it.stack = it.stack[:len(it.stack)-1]

	it.pushLeft(node.Right)

	return node.Order
}

// LevelOrderTraversal 执行层序遍历。
func (t *RBTree) LevelOrderTraversal() []*Order {
	if t.Root == nil {
		return nil
	}

	result := make([]*Order, 0, t.Size)
	queue := []*RBNode{t.Root}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		result = append(result, node.Order)

		if node.Left != nil {
			queue = append(queue, node.Left)
		}

		if node.Right != nil {
			queue = append(queue, node.Right)
		}
	}

	return result
}