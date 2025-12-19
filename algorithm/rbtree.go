// Package algos 提供红黑树数据结构和订单簿实现
package algorithm

// Color 红黑树节点颜色
type Color bool

const (
	Red   Color = true  // Red 红色节点
	Black Color = false // Black 黑色节点
)

// RBNode 红黑树节点
type RBNode struct {
	Order  *Order
	Left   *RBNode
	Right  *RBNode
	Parent *RBNode
	Color  Color
}

// RBTree 红黑树
type RBTree struct {
	Root *RBNode
	// true 表示最大堆（用于买单），false 表示最小堆（用于卖单）
	// 注意：这里借用“堆”的概念，实际上是排序方向
	IsMaxTree bool
	Size      int
}

// NewRBTree 创建红黑树
func NewRBTree(isMaxTree bool) *RBTree {
	return &RBTree{
		IsMaxTree: isMaxTree,
	}
}

// Compare 比较两个订单
// 返回 -1 (a < b), 0 (a == b), 1 (a > b)
func (t *RBTree) Compare(a, b *Order) int {
	cmp := a.Price.Cmp(b.Price)
	if cmp == 0 {
		// 价格相同时，按时间优先（时间戳小的优先）
		if a.Timestamp < b.Timestamp {
			// 对于买单和卖单，时间优先的逻辑是一样的：先来的排在前面
			// 在树中，我们希望“优”的排在前面（比如左边或右边，取决于遍历顺序）
			// 为了统一，我们定义：
			// 买单：价格高优先，时间早优先
			// 卖单：价格低优先，时间早优先

			// 这里仅比较大小关系，具体的优先顺序由 IsMaxTree 和遍历方向决定
			// 假设我们总是希望：
			// 买单树：左子树 > 右子树（降序），或者 右子树 > 左子树（升序）
			// 为了简化，我们统一使用“小于”语义构建树，然后根据 IsMaxTree 选择遍历方向

			// 让我们定义标准的比较：
			// 如果 a "优于" b，则返回 1
			// 如果 a "劣于" b，则返回 -1
			// 但红黑树通常基于 Key 的自然顺序。

			// 让我们使用自然顺序：
			// Key = (Price, Timestamp)
			// 比较逻辑：
			// if Price != other.Price return Price.Cmp(other.Price)
			// return other.Timestamp - Timestamp (注意时间戳越小越优先，所以反过来？不，保持自然序)

			return -1
		} else if a.Timestamp > b.Timestamp {
			return 1
		}
		return 0
	}
	return cmp
}

// Less 判断 a 是否比 b 优先级更高 (返回 true)
func (t *RBTree) Less(a, b *Order) bool {
	cmp := a.Price.Cmp(b.Price)
	if cmp == 0 {
		// 价格相同，时间戳小的优先（排在前面）
		return a.Timestamp < b.Timestamp
	}

	if t.IsMaxTree {
		// 买单：价格高的优先（排在前面）
		return cmp > 0
	}
	// 卖单：价格低的优先（排在前面）
	return cmp < 0
}

// Insert 插入订单
func (t *RBTree) Insert(order *Order) {
	z := &RBNode{Order: order, Color: Red}
	var y *RBNode
	x := t.Root

	for x != nil {
		y = x
		if t.Less(z.Order, x.Order) {
			x = x.Left
		} else {
			x = x.Right
		}
	}

	z.Parent = y
	if y == nil {
		t.Root = z
	} else if t.Less(z.Order, y.Order) {
		y.Left = z
	} else {
		y.Right = z
	}

	t.insertFixup(z)
	t.Size++
}

// insertFixup 插入后修复红黑树性质
func (t *RBTree) insertFixup(z *RBNode) {
	for z.Parent != nil && z.Parent.Color == Red {
		if z.Parent == z.Parent.Parent.Left {
			y := z.Parent.Parent.Right
			if y != nil && y.Color == Red {
				z.Parent.Color = Black
				y.Color = Black
				z.Parent.Parent.Color = Red
				z = z.Parent.Parent
			} else {
				if z == z.Parent.Right {
					z = z.Parent
					t.leftRotate(z)
				}
				z.Parent.Color = Black
				z.Parent.Parent.Color = Red
				t.rightRotate(z.Parent.Parent)
			}
		} else {
			y := z.Parent.Parent.Left
			if y != nil && y.Color == Red {
				z.Parent.Color = Black
				y.Color = Black
				z.Parent.Parent.Color = Red
				z = z.Parent.Parent
			} else {
				if z == z.Parent.Left {
					z = z.Parent
					t.rightRotate(z)
				}
				z.Parent.Color = Black
				z.Parent.Parent.Color = Red
				t.leftRotate(z.Parent.Parent)
			}
		}
	}
	t.Root.Color = Black
}

// leftRotate 左旋操作
func (t *RBTree) leftRotate(x *RBNode) {
	y := x.Right
	x.Right = y.Left
	if y.Left != nil {
		y.Left.Parent = x
	}
	y.Parent = x.Parent
	if x.Parent == nil {
		t.Root = y
	} else if x == x.Parent.Left {
		x.Parent.Left = y
	} else {
		x.Parent.Right = y
	}
	y.Left = x
	x.Parent = y
}

// rightRotate 右旋操作
func (t *RBTree) rightRotate(y *RBNode) {
	x := y.Left
	y.Left = x.Right
	if x.Right != nil {
		x.Right.Parent = y
	}
	x.Parent = y.Parent
	if y.Parent == nil {
		t.Root = x
	} else if y == y.Parent.Right {
		y.Parent.Right = x
	} else {
		y.Parent.Left = x
	}
	x.Right = y
	y.Parent = x
}

// Delete 删除订单
func (t *RBTree) Delete(order *Order) {
	z := t.find(order)
	if z == nil {
		return
	}
	t.deleteNode(z)
	t.Size--
}

// find 查找指定订单的节点
func (t *RBTree) find(order *Order) *RBNode {
	x := t.Root
	for x != nil {
		if x.Order.OrderID == order.OrderID {
			return x
		}
		if t.Less(order, x.Order) {
			x = x.Left
		} else {
			x = x.Right
		}
	}
	return nil
}

// deleteNode 删除指定节点
func (t *RBTree) deleteNode(z *RBNode) {
	var y, x *RBNode
	if z.Left == nil || z.Right == nil {
		y = z
	} else {
		y = t.successor(z)
	}

	if y.Left != nil {
		x = y.Left
	} else {
		x = y.Right
	}

	if x != nil {
		x.Parent = y.Parent
	}

	if y.Parent == nil {
		t.Root = x
	} else if y == y.Parent.Left {
		y.Parent.Left = x
	} else {
		y.Parent.Right = x
	}

	if y != z {
		z.Order = y.Order
	}

	if y.Color == Black && x != nil {
		t.deleteFixup(x)
	}
}

// successor 查找后继节点
func (t *RBTree) successor(x *RBNode) *RBNode {
	if x.Right != nil {
		return t.minimum(x.Right)
	}
	y := x.Parent
	for y != nil && x == y.Right {
		x = y
		y = y.Parent
	}
	return y
}

// minimum 查找子树中的最小节点
func (t *RBTree) minimum(x *RBNode) *RBNode {
	for x.Left != nil {
		x = x.Left
	}
	return x
}

// deleteFixup 删除后修复红黑树性质
func (t *RBTree) deleteFixup(x *RBNode) {
	for x != t.Root && x.Color == Black {
		if x == x.Parent.Left {
			w := x.Parent.Right
			if w.Color == Red {
				w.Color = Black
				x.Parent.Color = Red
				t.leftRotate(x.Parent)
				w = x.Parent.Right
			}
			if (w.Left == nil || w.Left.Color == Black) && (w.Right == nil || w.Right.Color == Black) {
				w.Color = Red
				x = x.Parent
			} else {
				if w.Right == nil || w.Right.Color == Black {
					if w.Left != nil {
						w.Left.Color = Black
					}
					w.Color = Red
					t.rightRotate(w)
					w = x.Parent.Right
				}
				w.Color = x.Parent.Color
				x.Parent.Color = Black
				if w.Right != nil {
					w.Right.Color = Black
				}
				t.leftRotate(x.Parent)
				x = t.Root
			}
		} else {
			w := x.Parent.Left
			if w.Color == Red {
				w.Color = Black
				x.Parent.Color = Red
				t.rightRotate(x.Parent)
				w = x.Parent.Left
			}
			if (w.Right == nil || w.Right.Color == Black) && (w.Left == nil || w.Left.Color == Black) {
				w.Color = Red
				x = x.Parent
			} else {
				if w.Left == nil || w.Left.Color == Black {
					if w.Right != nil {
						w.Right.Color = Black
					}
					w.Color = Red
					t.leftRotate(w)
					w = x.Parent.Left
				}
				w.Color = x.Parent.Color
				x.Parent.Color = Black
				if w.Left != nil {
					w.Left.Color = Black
				}
				t.rightRotate(x.Parent)
				x = t.Root
			}
		}
	}
	x.Color = Black
}

// GetBest 获取最优订单
func (t *RBTree) GetBest() *Order {
	if t.Root == nil {
		return nil
	}
	// 由于 Less 定义了优先级（高优先级 < 低优先级），最优元素总是最左节点
	return t.minimum(t.Root).Order
}

// Iterator 迭代器
type Iterator struct {
	stack []*RBNode
}

// NewIterator 创建迭代器（中序遍历，即按优先级从高到低）
func (t *RBTree) NewIterator() *Iterator {
	it := &Iterator{stack: make([]*RBNode, 0)}
	it.pushLeft(t.Root)
	return it
}

// pushLeft 将节点及其所有左子节点压入栈
func (it *Iterator) pushLeft(x *RBNode) {
	for x != nil {
		it.stack = append(it.stack, x)
		x = x.Left
	}
}

// Next 返回下一个订单
func (it *Iterator) Next() *Order {
	if len(it.stack) == 0 {
		return nil
	}
	node := it.stack[len(it.stack)-1]
	it.stack = it.stack[:len(it.stack)-1]
	it.pushLeft(node.Right)
	return node.Order
}
