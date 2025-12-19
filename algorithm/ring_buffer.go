package algorithm

import (
	"errors"
	"sync/atomic"
)

// RingBuffer 是一个固定大小的循环缓冲区。
// 这个实现为了简单和正确性使用了原子操作。
// 注意：在没有额外同步的情况下，此实现是线程安全的，适用于单生产者-单消费者（SPSC）场景。
// 对于多生产者或多消费者（MPMC）场景，需要额外的锁机制或更复杂的无锁算法。
type RingBuffer[T any] struct {
	buffer []T
	size   uint64
	mask   uint64
	head   uint64 // 读取索引
	tail   uint64 // 写入索引
}

// ErrBufferFull 表示环形缓冲区已满。
var ErrBufferFull = errors.New("ring buffer is full")

// ErrBufferEmpty 表示环形缓冲区为空。
var ErrBufferEmpty = errors.New("ring buffer is empty")

// NewRingBuffer 创建一个具有给定容量的新 RingBuffer。
// 容量必须是 2 的幂，以利用位运算进行高效的索引计算。
func NewRingBuffer[T any](capacity uint64) (*RingBuffer[T], error) {
	if capacity == 0 || (capacity&(capacity-1)) != 0 {
		return nil, errors.New("capacity must be a power of 2")
	}
	return &RingBuffer[T]{
		buffer: make([]T, capacity),
		size:   capacity,
		mask:   capacity - 1, // 例如，容量为8，mask为7 (0111b)，用于 (index & mask)
	}, nil
}

// Offer 向缓冲区添加一个项目。
// 如果缓冲区已满，则返回 ErrBufferFull 错误。
func (rb *RingBuffer[T]) Offer(item T) error {
	tail := atomic.LoadUint64(&rb.tail) // 获取当前写入索引
	head := atomic.LoadUint64(&rb.head) // 获取当前读取索引

	// 检查缓冲区是否已满。tail - head 等于当前缓冲区中的元素数量。
	if tail-head >= rb.size {
		return ErrBufferFull
	}

	// 将项目写入到对应索引位置。使用位运算 (tail & rb.mask) 替代取模运算 (tail % rb.size)。
	rb.buffer[tail&rb.mask] = item
	// 原子地增加写入索引。
	atomic.StoreUint64(&rb.tail, tail+1)
	return nil
}

// Poll 从缓冲区移除并返回一个项目。
// 如果缓冲区为空，则返回该类型零值和 ErrBufferEmpty 错误。
func (rb *RingBuffer[T]) Poll() (T, error) {
	var zero T                          // 泛型T的零值
	tail := atomic.LoadUint64(&rb.tail) // 获取当前写入索引
	head := atomic.LoadUint64(&rb.head) // 获取当前读取索引

	// 检查缓冲区是否为空。head 等于或大于 tail 意味着没有可读取的元素。
	if head >= tail {
		return zero, ErrBufferEmpty
	}

	// 从对应索引位置读取项目。
	item := rb.buffer[head&rb.mask]
	// 原子地增加读取索引。
	atomic.StoreUint64(&rb.head, head+1)
	return item, nil
}
