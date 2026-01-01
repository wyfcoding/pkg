package algorithm

import (
	"math"
	"sort"
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

// KDTree K-D 树
type KDTree struct {
	Root *KDNode
	K    int
}

// NewKDTree 构建 K-D 树
func NewKDTree(points []KDPoint) *KDTree {
	if len(points) == 0 {
		return nil
	}
	k := len(points[0].Vector)
	tree := &KDTree{K: k}
	tree.Root = tree.build(points, 0)
	return tree
}

func (t *KDTree) build(points []KDPoint, depth int) *KDNode {
	if len(points) == 0 {
		return nil
	}

	axis := depth % t.K

	// 根据当前维度排序，取中位数
	sort.Slice(points, func(i, j int) bool {
		return points[i].Vector[axis] < points[j].Vector[axis]
	})

	mid := len(points) / 2
	return &KDNode{
		Point: points[mid],
		Axis:  axis,
		Left:  t.build(points[:mid], depth+1),
		Right: t.build(points[mid+1:], depth+1),
	}
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
