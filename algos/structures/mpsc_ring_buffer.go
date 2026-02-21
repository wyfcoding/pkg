package structures

import (
	"log/slog"
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/wyfcoding/pkg/xerrors"
)

// MpscRingBuffer 是一个多生产者单消费者（Multi-Producer Single-Consumer）的无锁环形缓冲区。
// 适用于：撮合引擎定序、日志收集、Actor 模型消息邮箱。
// 性能：在高并发写入下远超标准 Channel。
type MpscRingBuffer[T any] struct {
	buffer []unsafe.Pointer // 存储 *T，为了支持原子操作，必须存储指针。
	_      [40]byte         // Padding to isolate head.
	head   uint64           // 消费者读取位置 (仅由消费者修改)。
	_      [56]byte         // Padding to isolate tail.
	tail   uint64           // 生产者写入位置 (由多个生产者通过 CAS 修改)。
	_      [56]byte         // Padding to isolate mask.
	mask   uint64
}

// NewMpscRingBuffer 创建一个新的 MPSC RingBuffer。
func NewMpscRingBuffer[T any](capacity uint64) (*MpscRingBuffer[T], error) {
	if capacity == 0 || (capacity&(capacity-1)) != 0 {
		slog.Error("MpscRingBuffer initialization failed: capacity must be a power of 2", "capacity", capacity)
		return nil, xerrors.ErrCapacityPowerOf2
	}

	slog.Info("MpscRingBuffer initialized", "capacity", capacity)
	return &MpscRingBuffer[T]{
		mask:   capacity - 1,
		buffer: make([]unsafe.Pointer, capacity),
	}, nil
}

// Offer 生产者写入数据。
// 这是一个无锁操作，使用 CAS 循环直到成功预留位置。
// 注意：不支持写入 nil 元素，因为 nil 被用作“槽位为空”的标记。
func (rb *MpscRingBuffer[T]) Offer(item *T) bool {
	if item == nil {
		return false // 不允许写入 nil。
	}

	var tail, head uint64

	for {
		tail = atomic.LoadUint64(&rb.tail)
		head = atomic.LoadUint64(&rb.head)

		// 检查是否已满。
		// 注意：这里的 head 读取可能稍微滞后，但在 MPSC 中是安全的（只会导致假满）。
		if tail-head >= uint64(len(rb.buffer)) {
			return false // Buffer full。
		}

		// 尝试抢占 tail。
		if atomic.CompareAndSwapUint64(&rb.tail, tail, tail+1) {
			break
		}
		// CAS 失败，说明有其他生产者抢先了，重试。
		runtime.Gosched() // 让出 CPU，避免活锁。
	}

	// 写入数据。
	// 注意：这里存在一个极短的竞争窗口。虽然我们抢到了 tail 索引。
	// 但如果其他生产者抢到了 tail+1 并先完成了写入。
	// 消费者可能会读到这个位置是 nil (尚未写入)。
	// 因此消费者必须检查数据是否为 nil。
	atomic.StorePointer(&rb.buffer[tail&rb.mask], unsafe.Pointer(item))
	return true
}

// Poll 消费者读取数据。
// 仅单线程调用。
func (rb *MpscRingBuffer[T]) Poll() *T {
	head := rb.head
	tail := atomic.LoadUint64(&rb.tail) // 获取当前最大写入位置。

	if head >= tail {
		return nil // Empty。
	}

	// 读取数据。
	// 关键：必须检查该位置是否已经完成写入。
	// 因为 Offer 中 CAS 成功后到 StorePointer 之间有时间差。
	ptr := atomic.LoadPointer(&rb.buffer[head&rb.mask])
	if ptr == nil {
		return nil // 生产者虽然抢到了位置，但还没写完数据，视为暂时为空。
	}

	// 清理位置 (可选，为了 GC)。
	atomic.StorePointer(&rb.buffer[head&rb.mask], nil)

	// 移动 head。
	// 不需要原子操作，因为只有这一个消费者。
	atomic.StoreUint64(&rb.head, head+1)

	return (*T)(ptr)
}

// Size 返回当前大致的元素数量。
func (rb *MpscRingBuffer[T]) Size() uint64 {
	tail := atomic.LoadUint64(&rb.tail)
	head := atomic.LoadUint64(&rb.head)
	if tail < head {
		return 0
	}
	return tail - head
}
