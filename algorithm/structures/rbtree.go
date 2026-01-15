// Package structures 提供红黑树数据结构.
package structures

import (
	"cmp"
)

// Color 红黑树节点颜色.
type Color bool

const (
	// ColorRed 红色节点.
	ColorRed Color = true
	// ColorBlack 黑色节点.
	ColorBlack Color = false
)

// RBNode 红黑树节点.
type RBNode[T any] struct {
	Value  T
	Left   *RBNode[T]
	Right  *RBNode[T]
	Parent *RBNode[T]
	Color  Color
}

// Comparator 比较器函数原型.
type Comparator[T any] func(a, b T) int

// RBTree 红黑树数据结构实现.
type RBTree[T any] struct {
	Root    *RBNode[T]
	compare Comparator[T]
	Size    int
}

// NewRBTree 创建一个新的红黑树.
func NewRBTree[T any](comp Comparator[T]) *RBTree[T] {
	return &RBTree[T]{
		compare: comp,
		Root:    nil,
		Size:    0,
	}
}

// NewOrderedRBTree 为实现了 cmp.Ordered 的类型创建红黑树.
func NewOrderedRBTree[T cmp.Ordered]() *RBTree[T] {
	return NewRBTree(func(a, b T) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
}

// Insert 向红黑树中插入一个新值.
func (t *RBTree[T]) Insert(val T) *RBNode[T] {
	newNode := &RBNode[T]{Value: val, Color: ColorRed}
	var parentNode *RBNode[T]

	currNode := t.Root
	for currNode != nil {
		parentNode = currNode
		if t.compare(val, currNode.Value) < 0 {
			currNode = currNode.Left
		} else {
			currNode = currNode.Right
		}
	}

	newNode.Parent = parentNode
	switch {
	case parentNode == nil:
		t.Root = newNode
	case t.compare(val, parentNode.Value) < 0:
		parentNode.Left = newNode
	default:
		parentNode.Right = newNode
	}

	t.insertFixup(newNode)
	t.Size++

	return newNode
}

func (t *RBTree[T]) insertFixup(targetNode *RBNode[T]) {
	node := targetNode

	for node.Parent != nil && node.Parent.Color == ColorRed {
		if node.Parent.Parent != nil && node.Parent == node.Parent.Parent.Left {
			node = t.fixInsertSide(node, true)
		} else if node.Parent.Parent != nil {
			node = t.fixInsertSide(node, false)
		} else {
			break
		}
	}

	t.Root.Color = ColorBlack
}

func (t *RBTree[T]) fixInsertSide(node *RBNode[T], isLeft bool) *RBNode[T] {
	var uncle *RBNode[T]
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

func (t *RBTree[T]) leftRotate(nodeX *RBNode[T]) {
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

func (t *RBTree[T]) rightRotate(nodeY *RBNode[T]) {
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

// DeleteNode 从树中删除一个节点。
func (t *RBTree[T]) DeleteNode(target *RBNode[T]) {
	var replaceNode, childNode *RBNode[T]

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

	switch {
	case replaceNode.Parent == nil:
		t.Root = childNode
	case replaceNode == replaceNode.Parent.Left:
		replaceNode.Parent.Left = childNode
	default:
		replaceNode.Parent.Right = childNode
	}

	if replaceNode != target {
		target.Value = replaceNode.Value
	}

	if replaceNode.Color == ColorBlack && childNode != nil {
		t.deleteFixup(childNode)
	}

	t.Size--
}

func (t *RBTree[T]) successor(node *RBNode[T]) *RBNode[T] {
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

func (t *RBTree[T]) minimum(node *RBNode[T]) *RBNode[T] {
	curr := node
	for curr.Left != nil {
		curr = curr.Left
	}
	return curr
}

func (t *RBTree[T]) deleteFixup(targetNode *RBNode[T]) {
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

func (t *RBTree[T]) fixDeleteSide(node *RBNode[T], isLeft bool) *RBNode[T] {
	var sibling *RBNode[T]
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

func (t *RBTree[T]) fixDeleteSideAdvanced(node, sibling *RBNode[T], isLeft bool) *RBNode[T] {
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

// Min 返回最小值节点.
func (t *RBTree[T]) Min() *RBNode[T] {
	if t.Root == nil {
		return nil
	}
	return t.minimum(t.Root)
}

// Iterator 迭代器。
type Iterator[T any] struct {
	stack []*RBNode[T]
}

// NewIterator 创建一个新迭代器。
func (t *RBTree[T]) NewIterator() *Iterator[T] {
	it := &Iterator[T]{stack: make([]*RBNode[T], 0)}
	it.pushLeft(t.Root)
	return it
}

func (it *Iterator[T]) pushLeft(node *RBNode[T]) {
	for node != nil {
		it.stack = append(it.stack, node)
		node = node.Left
	}
}

// Next 返回下一个值。
func (it *Iterator[T]) Next() (T, bool) {
	if len(it.stack) == 0 {
		var zero T
		return zero, false
	}

	node := it.stack[len(it.stack)-1]
	it.stack = it.stack[:len(it.stack)-1]
	it.pushLeft(node.Right)

	return node.Value, true
}
