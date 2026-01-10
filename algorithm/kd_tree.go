package algorithm

import (
	"errors"
	"math"
	"sync"
)

// KDPoint 多维空间中的一个点
type KDPoint struct {
	ID     uint64
	Vector []float64
}

// KDNode K-D 树节点
type KDNode struct {
	Point KDPoint
	Left  *KDNode
	Right *KDNode
	Axis  int
}

var kdNodePool = sync.Pool{
	New: func() any {
		return &KDNode{}
	},
}

func newKDNode(p KDPoint, axis int) *KDNode {
	node := kdNodePool.Get().(*KDNode)
	node.Point = p
	node.Axis = axis
	node.Left = nil
	node.Right = nil
	return node
}

// KDTree K-D 树
type KDTree struct {
	Root *KDNode
	K    int
}

// NewKDTree 构建 K-D 树
func NewKDTree(points []KDPoint) (*KDTree, error) {
	if len(points) == 0 {
		return nil, errors.New("points must not be empty")
	}
	k := len(points[0].Vector)
	tree := &KDTree{K: k}
	tree.Root = tree.build(points, 0)
	return tree, nil
}

// build 使用 QuickSelect 思想实现 O(n log n) 构建
func (t *KDTree) build(points []KDPoint, depth int) *KDNode {
	if len(points) == 0 {
		return nil
	}

	axis := depth % t.K
	mid := len(points) / 2

	// 使用 QuickSelect 找到中位数所在位置的点，并对数组进行分区
	t.quickSelect(points, mid, axis)

	node := newKDNode(points[mid], axis)
	node.Left = t.build(points[:mid], depth+1)
	node.Right = t.build(points[mid+1:], depth+1)
	return node
}

// quickSelect 在 O(n) 时间内找到第 k 小的元素并对区间进行分区
func (t *KDTree) quickSelect(points []KDPoint, k int, axis int) {
	left, right := 0, len(points)-1
	for left < right {
		pivotIdx := t.partition(points, left, right, axis)
		if pivotIdx == k {
			return
		} else if pivotIdx < k {
			left = pivotIdx + 1
		} else {
			right = pivotIdx - 1
		}
	}
}

func (t *KDTree) partition(points []KDPoint, left, right, axis int) int {
	pivotVal := points[right].Vector[axis]
	i := left
	for j := left; j < right; j++ {
		if points[j].Vector[axis] < pivotVal {
			points[i], points[j] = points[j], points[i]
			i++
		}
	}
	points[i], points[right] = points[right], points[i]
	return i
}

// Nearest 寻找最近邻
func (t *KDTree) Nearest(target []float64) (KDPoint, float64) {
	var bestPoint KDPoint
	minDist := math.MaxFloat64
	t.search(t.Root, target, &bestPoint, &minDist)
	return bestPoint, math.Sqrt(minDist)
}

func (t *KDTree) search(node *KDNode, target []float64, bestPoint *KDPoint, minDist *float64) {
	if node == nil {
		return
	}

	dist := sqDist(node.Point.Vector, target)
	if dist < *minDist {
		*minDist = dist
		*bestPoint = node.Point
	}

	axis := node.Axis
	diff := target[axis] - node.Point.Vector[axis]

	// 优先进入目标点所在的一侧
	var near, far *KDNode
	if diff < 0 {
		near, far = node.Left, node.Right
	} else {
		near, far = node.Right, node.Left
	}

	t.search(near, target, bestPoint, minDist)

	// 回溯检查另一侧是否可能存在更近的点
	if diff*diff < *minDist {
		t.search(far, target, bestPoint, minDist)
	}
}

func sqDist(v1, v2 []float64) float64 {
	sum := 0.0
	for i := range v1 {
		d := v1[i] - v2[i]
		sum += d * d
	}
	return sum
}
