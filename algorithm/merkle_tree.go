package algorithm

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	hashSize    = 32
	nodeBufSize = 64
)

// MerkleNode 代表 Merkle Tree 的一个节点.
type MerkleNode struct {
	Left  *MerkleNode
	Right *MerkleNode
	Hash  []byte
}

// MerkleTree 代表 Merkle Tree 结构.
type MerkleTree struct {
	Root *MerkleNode
}

// NewMerkleNode 创建一个新的 Merkle 节点.
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	node := &MerkleNode{
		Left:  left,
		Right: right,
		Hash:  nil,
	}

	if left == nil && right == nil {
		hash := sha256.Sum256(data)
		node.Hash = hash[:]
	} else {
		var buf [nodeBufSize]byte
		copy(buf[:hashSize], left.Hash)
		copy(buf[hashSize:], right.Hash)
		hash := sha256.Sum256(buf[:])
		node.Hash = hash[:]
	}

	return node
}

// NewMerkleTree 根据数据切片构建 Merkle Tree.
func NewMerkleTree(data [][]byte) *MerkleTree {
	if len(data) == 0 {
		return nil
	}

	nodes := make([]*MerkleNode, 0, len(data))
	for _, d := range data {
		nodes = append(nodes, NewMerkleNode(nil, nil, d))
	}

	for len(nodes) > 1 {
		if len(nodes)%2 != 0 {
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		nextLevel := make([]*MerkleNode, 0, len(nodes)/2)
		for i := 0; i < len(nodes); i += 2 {
			node := NewMerkleNode(nodes[i], nodes[i+1], nil)
			nextLevel = append(nextLevel, node)
		}

		nodes = nextLevel
	}

	return &MerkleTree{Root: nodes[0]}
}

// RootHashHex 返回根哈希的十六进制表示.
func (m *MerkleTree) RootHashHex() string {
	if m == nil || m.Root == nil {
		return ""
	}

	return hex.EncodeToString(m.Root.Hash)
}
