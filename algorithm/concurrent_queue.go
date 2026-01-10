// Package algos - 并发安全的队列数据结构
package algorithm

import (
	"fmt"
	"log/slog"
	"sync"
)

// ConcurrentQueue 并发安全的队列
// 使用互斥锁保证线程安全
// 时间复杂度：入队 O(1)，出队 O(1) (amortized)
type ConcurrentQueue struct {
	mu    sync.Mutex
	items []any
}

// NewConcurrentQueue 创建并发队列
func NewConcurrentQueue() *ConcurrentQueue {
	slog.Info("ConcurrentQueue initialized")
	return &ConcurrentQueue{
		items: make([]any, 0),
	}
}

// Enqueue 入队
func (cq *ConcurrentQueue) Enqueue(item any) {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	cq.items = append(cq.items, item)
}

// Dequeue 出队
func (cq *ConcurrentQueue) Dequeue() (any, error) {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if len(cq.items) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	item := cq.items[0]
	// 防止内存泄漏：将引用置空，允许 GC 回收对象
	cq.items[0] = nil
	cq.items = cq.items[1:]

	// 缩容优化：如果长度小于容量的 1/4 且容量大于 256，则重新分配内存
	// 这可以防止底层数组头部由于切片操作而长期占用内存
	if n, c := len(cq.items), cap(cq.items); n > 0 && n < c/4 && c > 256 {
		newItems := make([]any, n, c/2)
		copy(newItems, cq.items)
		cq.items = newItems
	}

	return item, nil
}

// Peek 查看队首元素
func (cq *ConcurrentQueue) Peek() (any, error) {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if len(cq.items) == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	return cq.items[0], nil
}

// Size 获取队列大小
func (cq *ConcurrentQueue) Size() int {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	return len(cq.items)
}

// IsEmpty 检查队列是否为空
func (cq *ConcurrentQueue) IsEmpty() bool {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	return len(cq.items) == 0
}

// Clear 清空队列
func (cq *ConcurrentQueue) Clear() {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	// 重置时保留一个较小的容量，或者完全释放
	cq.items = make([]any, 0)
}

// ConcurrentStack 并发安全的栈
type ConcurrentStack struct {
	mu    sync.Mutex
	items []any
}

// NewConcurrentStack 创建并发栈
func NewConcurrentStack() *ConcurrentStack {
	slog.Info("ConcurrentStack initialized")
	return &ConcurrentStack{
		items: make([]any, 0),
	}
}

// Push 入栈
func (cs *ConcurrentStack) Push(item any) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.items = append(cs.items, item)
}

// Pop 出栈
func (cs *ConcurrentStack) Pop() (any, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	n := len(cs.items)
	if n == 0 {
		return nil, fmt.Errorf("stack is empty")
	}

	item := cs.items[n-1]
	// 防止内存泄漏
	cs.items[n-1] = nil
	cs.items = cs.items[:n-1]
	return item, nil
}

// Peek 查看栈顶元素
func (cs *ConcurrentStack) Peek() (any, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if len(cs.items) == 0 {
		return nil, fmt.Errorf("stack is empty")
	}

	return cs.items[len(cs.items)-1], nil
}

// Size 获取栈大小
func (cs *ConcurrentStack) Size() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.items)
}

// IsEmpty 检查栈是否为空
func (cs *ConcurrentStack) IsEmpty() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.items) == 0
}

// Clear 清空栈
func (cs *ConcurrentStack) Clear() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.items = make([]any, 0)
}

// ConcurrentRingBuffer 并发安全的环形缓冲区
// 用于高性能的固定大小缓冲
// 建议：对于极致性能场景，请优先使用 lockfree_queue.go 或 ring_buffer.go
type ConcurrentRingBuffer struct {
	mu       sync.Mutex
	buffer   []any
	capacity int
	head     int
	tail     int
	size     int
}

// NewConcurrentRingBuffer 创建并发环形缓冲区
func NewConcurrentRingBuffer(capacity int) *ConcurrentRingBuffer {
	slog.Info("ConcurrentRingBuffer initialized", "capacity", capacity)
	return &ConcurrentRingBuffer{
		buffer:   make([]any, capacity),
		capacity: capacity,
		head:     0,
		tail:     0,
		size:     0,
	}
}

// Write 写入数据
func (crb *ConcurrentRingBuffer) Write(item any) error {
	crb.mu.Lock()
	defer crb.mu.Unlock()

	if crb.size == crb.capacity {
		return fmt.Errorf("ring buffer is full")
	}

	crb.buffer[crb.tail] = item
	crb.tail = (crb.tail + 1) % crb.capacity
	crb.size++

	return nil
}

// Read 读取数据
func (crb *ConcurrentRingBuffer) Read() (any, error) {
	crb.mu.Lock()
	defer crb.mu.Unlock()

	if crb.size == 0 {
		return nil, fmt.Errorf("ring buffer is empty")
	}

	item := crb.buffer[crb.head]
	// 避免内存泄漏
	crb.buffer[crb.head] = nil
	crb.head = (crb.head + 1) % crb.capacity
	crb.size--

	return item, nil
}

// Size 获取缓冲区大小
func (crb *ConcurrentRingBuffer) Size() int {
	crb.mu.Lock()
	defer crb.mu.Unlock()
	return crb.size
}

// IsFull 检查缓冲区是否满
func (crb *ConcurrentRingBuffer) IsFull() bool {
	crb.mu.Lock()
	defer crb.mu.Unlock()
	return crb.size == crb.capacity
}

// IsEmpty 检查缓冲区是否为空
func (crb *ConcurrentRingBuffer) IsEmpty() bool {
	crb.mu.Lock()
	defer crb.mu.Unlock()
	return crb.size == 0
}

// Clear 清空缓冲区
func (crb *ConcurrentRingBuffer) Clear() {
	crb.mu.Lock()
	defer crb.mu.Unlock()
	// 重置所有元素为 nil
	for i := range crb.buffer {
		crb.buffer[i] = nil
	}
	crb.head = 0
	crb.tail = 0
	crb.size = 0
}
