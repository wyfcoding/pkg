package structures

import (
	"errors"
	"sync"
)

var (
	// ErrQueueEmpty 队列为空。
	ErrQueueEmpty = errors.New("queue is empty")
	// ErrStackEmpty 栈为空。
	ErrStackEmpty = errors.New("stack is empty")
	// ErrBufferFull 缓冲区已满。
	ErrBufferFull = errors.New("buffer is full")
)

// ConcurrentQueue 线程安全的队列实现。
type ConcurrentQueue[T any] struct {
	items []T
	mu    sync.RWMutex
}

// NewConcurrentQueue 创建一个新的线程安全队列。
func NewConcurrentQueue[T any]() *ConcurrentQueue[T] {
	return &ConcurrentQueue[T]{
		items: make([]T, 0),
	}
}

// Enqueue 将元素入队。
func (cq *ConcurrentQueue[T]) Enqueue(item T) {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	cq.items = append(cq.items, item)
}

// Dequeue 将元素出队。
func (cq *ConcurrentQueue[T]) Dequeue() (T, error) {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if len(cq.items) == 0 {
		var zero T
		return zero, ErrQueueEmpty
	}

	item := cq.items[0]
	cq.items = cq.items[1:]

	// 定期缩减切片容量以释放内存
	if n, c := len(cq.items), cap(cq.items); n > 0 && n <= c/4 {
		newItems := make([]T, n, c/2)
		copy(newItems, cq.items)
		cq.items = newItems
	}

	return item, nil
}

// Peek 返回队头元素但不移除。
func (cq *ConcurrentQueue[T]) Peek() (T, error) {
	cq.mu.RLock()
	defer cq.mu.RUnlock()

	if len(cq.items) == 0 {
		var zero T
		return zero, ErrQueueEmpty
	}

	return cq.items[0], nil
}

// Size 返回队列中的元素数量。
func (cq *ConcurrentQueue[T]) Size() int {
	cq.mu.RLock()
	defer cq.mu.RUnlock()
	return len(cq.items)
}

// IsEmpty 检查队列是否为空。
func (cq *ConcurrentQueue[T]) IsEmpty() bool {
	cq.mu.RLock()
	defer cq.mu.RUnlock()
	return len(cq.items) == 0
}

// Clear 清空队列。
func (cq *ConcurrentQueue[T]) Clear() {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	cq.items = make([]T, 0)
}

// ConcurrentStack 线程安全的栈实现。
type ConcurrentStack[T any] struct {
	items []T
	mu    sync.RWMutex
}

// NewConcurrentStack 创建一个新的线程安全栈。
func NewConcurrentStack[T any]() *ConcurrentStack[T] {
	return &ConcurrentStack[T]{
		items: make([]T, 0),
	}
}

// Push 将元素压入栈。
func (cs *ConcurrentStack[T]) Push(item T) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.items = append(cs.items, item)
}

// Pop 将元素弹出栈。
func (cs *ConcurrentStack[T]) Pop() (T, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	n := len(cs.items)
	if n == 0 {
		var zero T
		return zero, ErrStackEmpty
	}

	item := cs.items[n-1]
	cs.items = cs.items[:n-1]

	return item, nil
}

// Peek 返回栈顶元素但不移除。
func (cs *ConcurrentStack[T]) Peek() (T, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	n := len(cs.items)
	if n == 0 {
		var zero T
		return zero, ErrStackEmpty
	}

	return cs.items[n-1], nil
}

// Size 返回栈中的元素数量。
func (cs *ConcurrentStack[T]) Size() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.items)
}

// IsEmpty 检查栈是否为空。
func (cs *ConcurrentStack[T]) IsEmpty() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.items) == 0
}

// Clear 清空栈。
func (cs *ConcurrentStack[T]) Clear() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.items = make([]T, 0)
}

// ConcurrentRingBuffer 线程安全的环形缓冲区。
type ConcurrentRingBuffer[T any] struct {
	buffer   []T
	head     int
	tail     int
	size     int
	capacity int
	mu       sync.RWMutex
}

// NewConcurrentRingBuffer 创建一个新的环形缓冲区。
func NewConcurrentRingBuffer[T any](capacity int) *ConcurrentRingBuffer[T] {
	return &ConcurrentRingBuffer[T]{
		buffer:   make([]T, capacity),
		capacity: capacity,
	}
}

// Write 写入元素。如果已满则返回错误。
func (crb *ConcurrentRingBuffer[T]) Write(item T) error {
	crb.mu.Lock()
	defer crb.mu.Unlock()

	if crb.size == crb.capacity {
		return ErrBufferFull
	}

	crb.buffer[crb.tail] = item
	crb.tail = (crb.tail + 1) % crb.capacity
	crb.size++

	return nil
}

// Read 读取元素。如果为空则返回错误。
func (crb *ConcurrentRingBuffer[T]) Read() (T, error) {
	crb.mu.Lock()
	defer crb.mu.Unlock()

	if crb.size == 0 {
		var zero T
		return zero, ErrQueueEmpty
	}

	item := crb.buffer[crb.head]
	var zero T
	crb.buffer[crb.head] = zero // 释放引用
	crb.head = (crb.head + 1) % crb.capacity
	crb.size--

	return item, nil
}

// Size 返回当前缓冲区大小。
func (crb *ConcurrentRingBuffer[T]) Size() int {
	crb.mu.RLock()
	defer crb.mu.RUnlock()
	return crb.size
}
