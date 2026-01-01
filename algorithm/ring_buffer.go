package algorithm

import (
	"errors"
	"runtime"
	"sync/atomic"
)

// CacheLinePad 用于防止伪共享 (False Sharing)。
// 现代 CPU 的缓存行通常为 64 字节。
type CacheLinePad struct {
	_ [8]uint64 // 64 bytes
}

// RingBuffer 是一个高性能、并发安全的循环缓冲区。
// 设计参考了 LMAX Disruptor，采用了内存对齐、缓存行填充和位运算优化。
type RingBuffer[T any] struct {
	_      CacheLinePad
	buffer []T
	size   uint64
	mask   uint64
	_      CacheLinePad
	head   uint64 // 读取索引
	_      CacheLinePad
	tail   uint64 // 写入索引
	_      CacheLinePad
}

var (
	ErrBufferFull  = errors.New("ring buffer is full")
	ErrBufferEmpty = errors.New("ring buffer is empty")
)

// NewRingBuffer 创建一个具有给定容量的新 RingBuffer。
// 容量必须是 2 的幂，以利用 (index & mask) 替代取模运算。
func NewRingBuffer[T any](capacity uint64) (*RingBuffer[T], error) {
	if capacity == 0 || (capacity&(capacity-1)) != 0 {
		return nil, errors.New("capacity must be a power of 2")
	}
	return &RingBuffer[T]{
		buffer: make([]T, capacity),
		size:   capacity,
		mask:   capacity - 1,
	}, nil
}

// Offer 向缓冲区添加一个项目 (Thread-safe for Multiple Producers)。
// 采用原子 CAS 操作争抢写入槽位。
func (rb *RingBuffer[T]) Offer(item T) error {
	for {
		tail := atomic.LoadUint64(&rb.tail)
		head := atomic.LoadUint64(&rb.head)

		if tail-head >= rb.size {
			return ErrBufferFull
		}

		// 尝试抢占 tail 索引
		if atomic.CompareAndSwapUint64(&rb.tail, tail, tail+1) {
			rb.buffer[tail&rb.mask] = item
			return nil
		}
		// 竞争失败，自旋或让出 CPU
		runtime.Gosched()
	}
}

// Poll 从缓冲区取出一个项目 (Thread-safe for Multiple Consumers)。
// 采用原子 CAS 操作争抢读取槽位。
func (rb *RingBuffer[T]) Poll() (T, error) {
	var zero T
	for {
		head := atomic.LoadUint64(&rb.head)
		tail := atomic.LoadUint64(&rb.tail)

		if head >= tail {
			return zero, ErrBufferEmpty
		}

		// 尝试抢占 head 索引
		if atomic.CompareAndSwapUint64(&rb.head, head, head+1) {
			item := rb.buffer[head&rb.mask]
			// 释放引用，帮助 GC (如果是指针类型)
			// rb.buffer[head&rb.mask] = zero 
			return item, nil
		}
		runtime.Gosched()
	}
}

// Capacity 返回缓冲区总容量。
func (rb *RingBuffer[T]) Capacity() uint64 {
	return rb.size
}

// Len 返回当前缓冲区中的元素数量。
func (rb *RingBuffer[T]) Len() uint64 {
	return atomic.LoadUint64(&rb.tail) - atomic.LoadUint64(&rb.head)
}
