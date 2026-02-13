package batch

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type BufferConfig struct {
	MaxSize        int
	MaxWait        time.Duration
	MaxConcurrency int
	FlushOnClose   bool
}

type BufferItem[T any] struct {
	Data      T
	Timestamp time.Time
}

type BatchHandler[T any] func(ctx context.Context, items []T) error

type Buffer[T any] struct {
	config    BufferConfig
	handler   BatchHandler[T]
	items     []BufferItem[T]
	mu        sync.Mutex
	flushChan chan struct{}
	closeChan chan struct{}
	closed    atomic.Bool
	wg        sync.WaitGroup
	stats     BufferStats
}

type BufferStats struct {
	TotalItems    atomic.Int64
	TotalBatches  atomic.Int64
	TotalErrors   atomic.Int64
	CurrentSize   atomic.Int64
	LastFlushTime atomic.Int64
}

func NewBuffer[T any](config BufferConfig, handler BatchHandler[T]) *Buffer[T] {
	if config.MaxSize <= 0 {
		config.MaxSize = 100
	}
	if config.MaxWait <= 0 {
		config.MaxWait = time.Second
	}
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = 10
	}

	b := &Buffer[T]{
		config:    config,
		handler:   handler,
		items:     make([]BufferItem[T], 0, config.MaxSize),
		flushChan: make(chan struct{}, 1),
		closeChan: make(chan struct{}),
	}

	b.wg.Add(1)
	go b.run()

	return b
}

func (b *Buffer[T]) Add(ctx context.Context, item T) error {
	if b.closed.Load() {
		return ErrBufferClosed
	}

	b.mu.Lock()
	b.items = append(b.items, BufferItem[T]{
		Data:      item,
		Timestamp: time.Now(),
	})
	b.stats.CurrentSize.Store(int64(len(b.items)))
	shouldFlush := len(b.items) >= b.config.MaxSize
	b.mu.Unlock()

	if shouldFlush {
		select {
		case b.flushChan <- struct{}{}:
		default:
		}
	}

	return nil
}

func (b *Buffer[T]) AddBatch(ctx context.Context, items []T) error {
	if b.closed.Load() {
		return ErrBufferClosed
	}

	b.mu.Lock()
	for _, item := range items {
		b.items = append(b.items, BufferItem[T]{
			Data:      item,
			Timestamp: time.Now(),
		})
	}
	b.stats.CurrentSize.Store(int64(len(b.items)))
	shouldFlush := len(b.items) >= b.config.MaxSize
	b.mu.Unlock()

	if shouldFlush {
		select {
		case b.flushChan <- struct{}{}:
		default:
		}
	}

	return nil
}

func (b *Buffer[T]) Flush() error {
	return b.flushWithContext(context.Background())
}

func (b *Buffer[T]) FlushWithContext(ctx context.Context) error {
	return b.flushWithContext(ctx)
}

func (b *Buffer[T]) flushWithContext(ctx context.Context) error {
	b.mu.Lock()
	if len(b.items) == 0 {
		b.mu.Unlock()
		return nil
	}

	items := b.items
	b.items = make([]BufferItem[T], 0, b.config.MaxSize)
	b.stats.CurrentSize.Store(0)
	b.mu.Unlock()

	data := make([]T, len(items))
	for i, item := range items {
		data[i] = item.Data
	}

	err := b.handler(ctx, data)
	b.stats.TotalItems.Add(int64(len(data)))
	b.stats.TotalBatches.Add(1)
	b.stats.LastFlushTime.Store(time.Now().UnixNano())

	if err != nil {
		b.stats.TotalErrors.Add(1)
		return err
	}

	return nil
}

func (b *Buffer[T]) Close() error {
	if !b.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(b.closeChan)
	b.wg.Wait()

	if b.config.FlushOnClose {
		b.Flush()
	}

	return nil
}

func (b *Buffer[T]) Stats() BufferStats {
	return b.stats
}

func (b *Buffer[T]) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

func (b *Buffer[T]) run() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.config.MaxWait)
	defer ticker.Stop()

	for {
		select {
		case <-b.closeChan:
			return
		case <-b.flushChan:
			b.flushWithContext(context.Background())
		case <-ticker.C:
			b.flushWithContext(context.Background())
		}
	}
}

type AsyncBuffer[T any] struct {
	*Buffer[T]
	errorChan chan error
	semaphore chan struct{}
}

func NewAsyncBuffer[T any](config BufferConfig, handler BatchHandler[T]) *AsyncBuffer[T] {
	buf := NewBuffer[T](config, handler)

	asyncBuf := &AsyncBuffer[T]{
		Buffer:    buf,
		errorChan: make(chan error, 100),
		semaphore: make(chan struct{}, config.MaxConcurrency),
	}

	return asyncBuf
}

func (b *AsyncBuffer[T]) AddAsync(ctx context.Context, item T) <-chan error {
	resultChan := make(chan error, 1)

	go func() {
		defer close(resultChan)

		if err := b.Add(ctx, item); err != nil {
			resultChan <- err
		}
	}()

	return resultChan
}

func (b *AsyncBuffer[T]) Errors() <-chan error {
	return b.errorChan
}

type PartitionedBuffer[T any, K comparable] struct {
	config      BufferConfig
	handler     BatchHandler[T]
	partitioner func(T) K
	buffers     sync.Map
	closed      atomic.Bool
}

func NewPartitionedBuffer[T any, K comparable](
	config BufferConfig,
	handler BatchHandler[T],
	partitioner func(T) K,
) *PartitionedBuffer[T, K] {
	return &PartitionedBuffer[T, K]{
		config:      config,
		handler:     handler,
		partitioner: partitioner,
	}
}

func (b *PartitionedBuffer[T, K]) Add(ctx context.Context, item T) error {
	if b.closed.Load() {
		return ErrBufferClosed
	}

	key := b.partitioner(item)

	bufIface, _ := b.buffers.LoadOrStore(key, NewBuffer[T](b.config, b.handler))
	buf := bufIface.(*Buffer[T])

	return buf.Add(ctx, item)
}

func (b *PartitionedBuffer[T, K]) Flush() error {
	var lastErr error
	b.buffers.Range(func(key, value any) bool {
		if err := value.(*Buffer[T]).Flush(); err != nil {
			lastErr = err
		}
		return true
	})
	return lastErr
}

func (b *PartitionedBuffer[T, K]) Close() error {
	if !b.closed.CompareAndSwap(false, true) {
		return nil
	}

	var lastErr error
	b.buffers.Range(func(key, value any) bool {
		if err := value.(*Buffer[T]).Close(); err != nil {
			lastErr = err
		}
		return true
	})
	return lastErr
}

type PriorityBuffer[T any] struct {
	config    BufferConfig
	handler   BatchHandler[T]
	high      []BufferItem[T]
	normal    []BufferItem[T]
	low       []BufferItem[T]
	mu        sync.Mutex
	flushChan chan struct{}
	closeChan chan struct{}
	closed    atomic.Bool
	wg        sync.WaitGroup
}

type Priority int

const (
	PriorityHigh Priority = iota
	PriorityNormal
	PriorityLow
)

func NewPriorityBuffer[T any](config BufferConfig, handler BatchHandler[T]) *PriorityBuffer[T] {
	b := &PriorityBuffer[T]{
		config:    config,
		handler:   handler,
		high:      make([]BufferItem[T], 0, config.MaxSize/3),
		normal:    make([]BufferItem[T], 0, config.MaxSize/3),
		low:       make([]BufferItem[T], 0, config.MaxSize/3),
		flushChan: make(chan struct{}, 1),
		closeChan: make(chan struct{}),
	}

	b.wg.Add(1)
	go b.run()

	return b
}

func (b *PriorityBuffer[T]) Add(ctx context.Context, item T, priority Priority) error {
	if b.closed.Load() {
		return ErrBufferClosed
	}

	b.mu.Lock()
	bufferItem := BufferItem[T]{
		Data:      item,
		Timestamp: time.Now(),
	}

	switch priority {
	case PriorityHigh:
		b.high = append(b.high, bufferItem)
	case PriorityNormal:
		b.normal = append(b.normal, bufferItem)
	case PriorityLow:
		b.low = append(b.low, bufferItem)
	}

	totalSize := len(b.high) + len(b.normal) + len(b.low)
	shouldFlush := totalSize >= b.config.MaxSize
	b.mu.Unlock()

	if shouldFlush {
		select {
		case b.flushChan <- struct{}{}:
		default:
		}
	}

	return nil
}

func (b *PriorityBuffer[T]) Flush() error {
	return b.flushWithContext(context.Background())
}

func (b *PriorityBuffer[T]) flushWithContext(ctx context.Context) error {
	b.mu.Lock()
	if len(b.high) == 0 && len(b.normal) == 0 && len(b.low) == 0 {
		b.mu.Unlock()
		return nil
	}

	high := b.high
	normal := b.normal
	low := b.low
	b.high = make([]BufferItem[T], 0, b.config.MaxSize/3)
	b.normal = make([]BufferItem[T], 0, b.config.MaxSize/3)
	b.low = make([]BufferItem[T], 0, b.config.MaxSize/3)
	b.mu.Unlock()

	var allItems []T
	for _, item := range high {
		allItems = append(allItems, item.Data)
	}
	for _, item := range normal {
		allItems = append(allItems, item.Data)
	}
	for _, item := range low {
		allItems = append(allItems, item.Data)
	}

	if len(allItems) == 0 {
		return nil
	}

	return b.handler(ctx, allItems)
}

func (b *PriorityBuffer[T]) Close() error {
	if !b.closed.CompareAndSwap(false, true) {
		return nil
	}

	close(b.closeChan)
	b.wg.Wait()

	if b.config.FlushOnClose {
		b.Flush()
	}

	return nil
}

func (b *PriorityBuffer[T]) run() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.config.MaxWait)
	defer ticker.Stop()

	for {
		select {
		case <-b.closeChan:
			return
		case <-b.flushChan:
			b.flushWithContext(context.Background())
		case <-ticker.C:
			b.flushWithContext(context.Background())
		}
	}
}

type RetryBuffer[T any] struct {
	*Buffer[T]
	maxRetries int
	retryDelay time.Duration
}

func NewRetryBuffer[T any](
	config BufferConfig,
	handler BatchHandler[T],
	maxRetries int,
	retryDelay time.Duration,
) *RetryBuffer[T] {
	retryHandler := func(ctx context.Context, items []T) error {
		var lastErr error
		for i := 0; i <= maxRetries; i++ {
			if err := handler(ctx, items); err != nil {
				lastErr = err
				if i < maxRetries {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(retryDelay):
						continue
					}
				}
			} else {
				return nil
			}
		}
		return lastErr
	}

	return &RetryBuffer[T]{
		Buffer:     NewBuffer[T](config, retryHandler),
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}
}

type TransformBuffer[T, U any] struct {
	buffer    *Buffer[U]
	transform func(T) (U, error)
}

func NewTransformBuffer[T, U any](
	config BufferConfig,
	handler BatchHandler[U],
	transform func(T) (U, error),
) *TransformBuffer[T, U] {
	return &TransformBuffer[T, U]{
		buffer:    NewBuffer[U](config, handler),
		transform: transform,
	}
}

func (b *TransformBuffer[T, U]) Add(ctx context.Context, item T) error {
	transformed, err := b.transform(item)
	if err != nil {
		return err
	}
	return b.buffer.Add(ctx, transformed)
}

func (b *TransformBuffer[T, U]) Flush() error {
	return b.buffer.Flush()
}

func (b *TransformBuffer[T, U]) Close() error {
	return b.buffer.Close()
}

func (b *TransformBuffer[T, U]) Stats() BufferStats {
	return b.buffer.Stats()
}
