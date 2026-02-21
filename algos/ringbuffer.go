// 变更说明：实现高性能环形缓冲区（Disruptor 简化版）。
// 用于撮合引擎内部的指令分发和订单流流水线处理。
// 特性：无锁(Lock-free) CAS 操作，伪共享(False Sharing)衬垫处理。
package algos

import (
	"runtime"
	"sync/atomic"
)

type RingBuffer struct {
	_padding0 [8]uint64
	cursor    uint64 // 写入游标
	_padding1 [8]uint64
	size      uint64
	mask      uint64
	data      []interface{}
}

func NewRingBuffer(size uint64) *RingBuffer {
	if size == 0 || (size&(size-1)) != 0 {
		panic("size must be power of 2")
	}
	return &RingBuffer{
		size: size,
		mask: size - 1,
		data: make([]interface{}, size),
	}
}

func (rb *RingBuffer) Publish(val interface{}) {
	idx := atomic.AddUint64(&rb.cursor, 1) - 1
	rb.data[idx&rb.mask] = val
}

func (rb *RingBuffer) Get(pos uint64) interface{} {
	for atomic.LoadUint64(&rb.cursor) <= pos {
		runtime.Gosched() // 自旋等待
	}
	return rb.data[pos&rb.mask]
}
