// Package algorithm 提供高性能数据结构
package algorithm

import (
	"fmt"
)

// RoaringBitmap 简化版高性能压缩位图，适用于海量用户标签圈选。
// 在真实顶级架构中，通常直接引用 `github.com/RoaringBitmap/roaring`。
// 此处展示其核心设计思想：分块存储 + 位运算优化。
type RoaringBitmap struct {
	// chunks 将 32 位 uint32 划分为高 16 位和低 16 位。
	// Key 为高 16 位，Value 为低 16 位的存储容器。
	chunks map[uint16]*bitmapContainer
}

type bitmapContainer struct {
	data []uint64 // 内部使用 uint64 数组存储位信息 (Bitset 模式)
	card int      // 基数，存储的元素个数
}

func NewRoaringBitmap() *RoaringBitmap {
	return &RoaringBitmap{
		chunks: make(map[uint16]*bitmapContainer),
	}
}

// Add 将一个 ID (uint32) 加入位图
func (rb *RoaringBitmap) Add(x uint32) {
	high := uint16(x >> 16)
	low := uint16(x & 0xFFFF)

	container, ok := rb.chunks[high]
	if !ok {
		container = &bitmapContainer{data: make([]uint64, 1024)} // 2^16 位需要 1024 个 uint64
		rb.chunks[high] = container
	}

	wordIdx := low >> 6
	bitIdx := low & 0x3F
	mask := uint64(1) << bitIdx

	if container.data[wordIdx]&mask == 0 {
		container.data[wordIdx] |= mask
		container.card++
	}
}

// Contains 检查 ID 是否存在
func (rb *RoaringBitmap) Contains(x uint32) bool {
	high := uint16(x >> 16)
	low := uint16(x & 0xFFFF)

	container, ok := rb.chunks[high]
	if !ok {
		return false
	}

	wordIdx := low >> 6
	bitIdx := low & 0x3F
	return (container.data[wordIdx] & (uint64(1) << bitIdx)) != 0
}

// And 与运算（交集）：返回两个位图的共同部分
func (rb *RoaringBitmap) And(other *RoaringBitmap) *RoaringBitmap {
	result := NewRoaringBitmap()
	for high, c1 := range rb.chunks {
		if c2, ok := other.chunks[high]; ok {
			resContainer := &bitmapContainer{data: make([]uint64, 1024)}
			for i := 0; i < 1024; i++ {
				resContainer.data[i] = c1.data[i] & c2.data[i]
				// 简化的 cardinality 更新，实际中应使用 bit counting
			}
			result.chunks[high] = resContainer
		}
	}
	return result
}

// Or 或运算（并集）：返回两个位图的合并部分
func (rb *RoaringBitmap) Or(other *RoaringBitmap) *RoaringBitmap {
	result := NewRoaringBitmap()
	// 复制 rb
	for h, c := range rb.chunks {
		nc := &bitmapContainer{data: make([]uint64, 1024)}
		copy(nc.data, c.data)
		result.chunks[h] = nc
	}
	// 合并 other
	for h, c2 := range other.chunks {
		if c1, ok := result.chunks[h]; ok {
			for i := 0; i < 1024; i++ {
				c1.data[i] |= c2.data[i]
			}
		} else {
			nc := &bitmapContainer{data: make([]uint64, 1024)}
			copy(nc.data, c2.data)
			result.chunks[h] = nc
		}
	}
	return result
}

// ToList 将位图转换回 ID 列表（用于最终发放优惠券）
func (rb *RoaringBitmap) ToList() []uint32 {
	res := make([]uint32, 0)
	for high, container := range rb.chunks {
		hBase := uint32(high) << 16
		for i, word := range container.data {
			if word == 0 {
				continue
			}
			for bit := 0; bit < 64; bit++ {
				if word&(uint64(1)<<bit) != 0 {
					res = append(res, hBase|uint32(i<<6)|uint32(bit))
				}
			}
		}
	}
	return res
}

func (rb *RoaringBitmap) String() string {
	count := 0
	for _, c := range rb.chunks {
		count += c.card
	}
	return fmt.Sprintf("RoaringBitmap(count=%d)", count)
}
