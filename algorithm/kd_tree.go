package algorithm

import (
	"errors"
	"math"
	"sync"
)

var (
	// ErrEmptyPoints 输入点集不能为空.
	ErrEmptyPoints = errors.New("points must not be empty")
)

// KDPoint 多维空间中的一个点.
type KDPoint struct {
	Vector []float64
	ID     uint64
}

// KDNode K-D 树节点.
type KDNode struct {
	Left  *KDNode
	Right *KDNode
	Point KDPoint
	Axis  int
}

var kdNodePool = sync.Pool{
	New: func() any {
		return &KDNode{
			Point: KDPoint{
				Vector: nil,
				ID:     0,
			},
			Left:  nil,
			Right: nil,
			Axis:  0,
		}
	},
}

func acquireKDNode(p KDPoint, axis int) *KDNode {
	node, ok := kdNodePool.Get().(*KDNode)
	if !ok {
		return &KDNode{Point: p, Axis: axis, Left: nil, Right: nil}
	}

	node.Point = p
	node.Axis = axis
	node.Left = nil
	node.Right = nil

	return node
}

// KDTree K-D 树结构.
type KDTree struct {
	Root *KDNode
	K    int
}

// NewKDTree 构建 K-D 树.
func NewKDTree(points []KDPoint) (*KDTree, error) {
	if len(points) == 0 {
		return nil, ErrEmptyPoints
	}

	dim := len(points[0].Vector)
	tree := &KDTree{
		K:    dim,
		Root: nil,
	}
	tree.Root = tree.build(points, 0)

	return tree, nil
}

func (t *KDTree) build(points []KDPoint, depth int) *KDNode {
	if len(points) == 0 {
		return nil
	}

	axis := depth % t.K
	mid := len(points) / 2

	t.quickSelect(points, mid, axis)

	node := acquireKDNode(points[mid], axis)
	node.Left = t.build(points[:mid], depth+1)
	node.Right = t.build(points[mid+1:], depth+1)

	return node
}

func (t *KDTree) quickSelect(points []KDPoint, k, axis int) {
	left, right := 0, len(points)-1
	for left < right {
		pivotIdx := t.partition(points, left, right, axis)
		switch {
		case pivotIdx == k:
			return
		case pivotIdx < k:
			left = pivotIdx + 1
		default:
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

// Nearest 寻找最近邻点.
func (t *KDTree) Nearest(target []float64) (KDPoint, float64) {
	var best KDPoint
	minDist := math.MaxFloat64
	t.search(t.Root, target, &best, &minDist)

	return best, math.Sqrt(minDist)
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

	var near, far *KDNode
	if diff < 0 {
		near, far = node.Left, node.Right
	} else {
		near, far = node.Right, node.Left
	}

	t.search(near, target, bestPoint, minDist)

	if diff*diff < *minDist {
		t.search(far, target, bestPoint, minDist)
	}
}

func sqDist(v1, v2 []float64) float64 {
	var sum float64
	for i := range v1 {
		diff := v1[i] - v2[i]
		sum += diff * diff
	}

	return sum
}
