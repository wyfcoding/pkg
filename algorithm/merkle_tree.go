package algorithm

import (
	"crypto/sha256"
	"fmt"
)

// MerkleNode 代表 Merkle Tree 的一个节点。
type MerkleNode struct {
	Left  *MerkleNode
	Right *MerkleNode
	Hash  []byte
}

// MerkleTree 代表 Merkle Tree 结构。
// 适用于：数据完整性校验、审计日志防篡改。
type MerkleTree struct {
	Root *MerkleNode
}

// NewMerkleNode 创建一个新的 Merkle 节点。
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	node := &MerkleNode{}

	if left == nil && right == nil {
		// 叶子节点。
		hash := sha256.Sum256(data)
		node.Hash = hash[:]
	} else {
		// 中间节点：连接左右子节点的哈希值并再次哈希。
		// 优化：使用栈上数组避免 append 造成的堆分配。
		var buf [64]byte
		copy(buf[:32], left.Hash)
		copy(buf[32:], right.Hash)
		hash := sha256.Sum256(buf[:])
		node.Hash = hash[:]
	}

	node.Left = left
	node.Right = right

	return node
}

// NewMerkleTree 根据数据切片构建 Merkle Tree。
func NewMerkleTree(data [][]byte) *MerkleTree {
	var nodes []*MerkleNode

	// 处理空数组。
	if len(data) == 0 {
		return nil
	}

	// 1. 创建所有叶子节点。
	for _, d := range data {
		nodes = append(nodes, NewMerkleNode(nil, nil, d))
	}

	// 2. 递归向上构建父节点。
	for len(nodes) > 1 {
		// 如果叶子数量是奇数，复制最后一个节点凑成偶数。
		if len(nodes)%2 != 0 {
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		// 预分配下一层节点的切片容量，避免扩容开销。
		nextLevel := make([]*MerkleNode, 0, len(nodes)/2)
		for i := 0; i < len(nodes); i += 2 {
			node := NewMerkleNode(nodes[i], nodes[i+1], nil)
			nextLevel = append(nextLevel, node)
		}
		nodes = nextLevel
	}

	return &MerkleTree{Root: nodes[0]}
}

// RootHashHex 返回根哈希的十六进制表示。
func (m *MerkleTree) RootHashHex() string {
	if m == nil || m.Root == nil {
		return ""
	}
	return fmt.Sprintf("%x", m.Root.Hash)
}
