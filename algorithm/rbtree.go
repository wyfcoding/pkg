// Package algorithm 提供红黑树数据结构和订单簿实现.
package algorithm

import "sync"

// Color 红黑树节点颜色.
type Color bool

const (
	// ColorRed 红色节点.
	ColorRed Color = true
	// ColorBlack 黑色节点.
	ColorBlack Color = false
)

// RBNode 红黑树节点.
type RBNode struct {
	Order  *Order
	Left   *RBNode
	Right  *RBNode
	Parent *RBNode
	Color  Color
}

// RBTree 红黑树数据结构实现.
type RBTree struct {
	Root      *RBNode
	Size      int
	IsMaxTree bool
}

var rbNodePool = sync.Pool{
	New: func() any {
		return &RBNode{
			Order:  nil,
			Left:   nil,
			Right:  nil,
			Parent: nil,
			Color:  ColorBlack,
		}
	},
}

func newRBNode(order *Order, color Color) *RBNode {
	node, ok := rbNodePool.Get().(*RBNode)
	if !ok {
		return &RBNode{
			Order:  order,
			Color:  color,
			Left:   nil,
			Right:  nil,
			Parent: nil,
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

// NewRBTree 创建并返回一个指定排序方向的红黑树.
func NewRBTree(isMaxTree bool) *RBTree {
	return &RBTree{
		IsMaxTree: isMaxTree,
		Root:      nil,
		Size:      0,
	}
}

// Less 决定节点 a 是否应该排在节点 b 的左侧.
func (t *RBTree) Less(nodeA, nodeB *Order) bool {
	priceCmp := nodeA.Price.Cmp(nodeB.Price)
	if priceCmp == 0 {
		return nodeA.Timestamp < nodeB.Timestamp
	}

	if t.IsMaxTree {
		return priceCmp > 0
	}

	return priceCmp < 0
}

// Insert 向红黑树中插入一个新的订单.
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
	switch {
	case parentNode == nil:
		t.Root = newNode
	case t.Less(newNode.Order, parentNode.Order):
		parentNode.Left = newNode
	default:
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
			node = t.fixInsertSide(node, true)
		} else {
			node = t.fixInsertSide(node, false)
		}
	}

	t.Root.Color = ColorBlack
}

func (t *RBTree) fixInsertSide(node *RBNode, isLeft bool) *RBNode {
	var uncle *RBNode
	if isLeft {
		uncle = node.Parent.Parent.Right
	} else {
		uncle = node.Parent.Parent.Left
	}

	if uncle != nil && uncle.Color == ColorRed {
		node.Parent.Color = ColorBlack
		uncle.Color = ColorBlack
		node.Parent.Parent.Color = ColorRed

		return node.Parent.Parent
	}

	curr := node
	if isLeft {
		if curr == node.Parent.Right {
			curr = node.Parent
			t.leftRotate(curr)
		}
		curr.Parent.Color = ColorBlack
		curr.Parent.Parent.Color = ColorRed
		t.rightRotate(curr.Parent.Parent)
	} else {
		if curr == node.Parent.Left {
			curr = node.Parent
			t.rightRotate(curr)
		}
		curr.Parent.Color = ColorBlack
		curr.Parent.Parent.Color = ColorRed
		t.leftRotate(curr.Parent.Parent)
	}

	return curr
}

func (t *RBTree) leftRotate(nodeX *RBNode) {
	nodeY := nodeX.Right
	nodeX.Right = nodeY.Left

	if nodeY.Left != nil {
		nodeY.Left.Parent = nodeX
	}

	nodeY.Parent = nodeX.Parent
	switch {
	case nodeX.Parent == nil:
		t.Root = nodeY
	case nodeX == nodeX.Parent.Left:
		nodeX.Parent.Left = nodeY
	default:
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
	switch {
	case nodeY.Parent == nil:
		t.Root = nodeX
	case nodeY == nodeY.Parent.Right:
		nodeY.Parent.Right = nodeX
	default:
		nodeY.Parent.Left = nodeX
	}

	nodeX.Right = nodeY
	nodeY.Parent = nodeX
}

// Delete 根据 Order 寻找并删除节点.
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

// DeleteNode 删除指定节点.
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
			node = t.fixDeleteSide(node, true)
		} else {
			node = t.fixDeleteSide(node, false)
		}
	}

	node.Color = ColorBlack
}

func (t *RBTree) fixDeleteSide(node *RBNode, isLeft bool) *RBNode {
	var sibling *RBNode
	if isLeft {
		sibling = node.Parent.Right
	} else {
		sibling = node.Parent.Left
	}

	if sibling.Color == ColorRed {
		sibling.Color = ColorBlack
		node.Parent.Color = ColorRed
		if isLeft {
			t.leftRotate(node.Parent)
			sibling = node.Parent.Right
		} else {
			t.rightRotate(node.Parent)
			sibling = node.Parent.Left
		}
	}

	if (sibling.Left == nil || sibling.Left.Color == ColorBlack) &&
		(sibling.Right == nil || sibling.Right.Color == ColorBlack) {
		sibling.Color = ColorRed

		return node.Parent
	}

	return t.fixDeleteSideAdvanced(node, sibling, isLeft)
}

func (t *RBTree) fixDeleteSideAdvanced(node, sibling *RBNode, isLeft bool) *RBNode {
	sib := sibling
	if isLeft {
		if sib.Right == nil || sib.Right.Color == ColorBlack {
			if sib.Left != nil {
				sib.Left.Color = ColorBlack
			}
			sib.Color = ColorRed
			t.rightRotate(sib)
			sib = node.Parent.Right
		}
		sib.Color = node.Parent.Color
		node.Parent.Color = ColorBlack
		if sib.Right != nil {
			sib.Right.Color = ColorBlack
		}
		t.leftRotate(node.Parent)
	} else {
		if sib.Left == nil || sib.Left.Color == ColorBlack {
			if sib.Right != nil {
				sib.Right.Color = ColorBlack
			}
			sib.Color = ColorRed
			t.leftRotate(sib)
			sib = node.Parent.Left
		}
		sib.Color = node.Parent.Color
		node.Parent.Color = ColorBlack
		if sib.Left != nil {
			sib.Left.Color = ColorBlack
		}
		t.rightRotate(node.Parent)
	}

	return t.Root
}

// GetBest 获取最优订单.
func (t *RBTree) GetBest() *Order {
	if t.Root == nil {
		return nil
	}

	return t.minimum(t.Root).Order
}

// Iterator 迭代器.
type Iterator struct {
	stack []*RBNode
}

// NewIterator 创建迭代器.
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

// Next 返回下一个订单.
func (it *Iterator) Next() *Order {
	if len(it.stack) == 0 {
		return nil
	}

	node := it.stack[len(it.stack)-1]
	it.stack = it.stack[:len(it.stack)-1]

	it.pushLeft(node.Right)

	return node.Order
}

// LevelOrderTraversal 执行层序遍历.
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
