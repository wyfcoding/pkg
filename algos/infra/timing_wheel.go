package infra

import (
	"sync"
	"time"

	"github.com/sourcegraph/conc"
	"github.com/wyfcoding/pkg/xerrors"
)

// TimerTask 定义定时任务函数签名。
type TimerTask func()

// timerEntry 内部任务包装 (Singly Linked List Node)。
type timerEntry struct {
	expiration time.Time   // 绝对过期时间 (24 bytes)。
	task       TimerTask   // 任务回调 (8 bytes)。
	next       *timerEntry // 下一个节点 (8 bytes)。
	circle     int         // 剩余圈数 (8 bytes)。
}

// TimingWheel 是一个基于“圈数”的单层时间轮实现。
// 优化：
// 1. 移除 container/list，使用内嵌链表节点，减少指针跳转和内存占用。
// 2. 使用 sync.Pool 复用 timerEntry，实现零分配 (Zero Allocation) 添加任务。
// 3. 字段按大小排序以减少内存对齐填充。
type TimingWheel struct {
	pool      *sync.Pool      // 8 bytes
	wg        *conc.WaitGroup // 8 bytes
	exitC     chan struct{}   // 8 bytes
	slots     []*timerEntry   // 24 bytes (Pointer at offset 0, followed by Len/Cap)
	mu        sync.Mutex      // 8 bytes
	tick      time.Duration   // 8 bytes
	interval  time.Duration   // 8 bytes
	wheelSize int             // 8 bytes
	current   int             // 8 bytes
	running   bool            // 1 byte
}

// NewTimingWheel 创建一个新的时间.
func NewTimingWheel(tick time.Duration, wheelSize int) (*TimingWheel, error) {
	if tick <= 0 || wheelSize <= 0 {
		return nil, xerrors.ErrInvalidConfig
	}

	return &TimingWheel{
		pool: &sync.Pool{
			New: func() any {
				return &timerEntry{}
			},
		},
		wg:        &conc.WaitGroup{},
		exitC:     make(chan struct{}),
		slots:     make([]*timerEntry, wheelSize),
		tick:      tick,
		wheelSize: wheelSize,
		interval:  tick * time.Duration(wheelSize),
	}, nil
}

// Start 启动时间轮。
func (tw *TimingWheel) Start() {
	tw.mu.Lock()
	if tw.running {
		tw.mu.Unlock()
		return
	}
	tw.running = true
	tw.mu.Unlock()

	tw.wg.Go(func() {
		ticker := time.NewTicker(tw.tick)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				tw.tickTock()
			case <-tw.exitC:
				return
			}
		}
	})
}

// Stop 停止时间轮。
func (tw *TimingWheel) Stop() {
	tw.mu.Lock()
	if !tw.running {
		tw.mu.Unlock()
		return
	}
	tw.running = false
	tw.mu.Unlock()

	close(tw.exitC)
	tw.wg.Wait()
}

// AddTask 添加一个延迟任务。
func (tw *TimingWheel) AddTask(delay time.Duration, task TimerTask) error {
	if delay < 0 {
		return xerrors.ErrInvalidDelay
	}

	tw.mu.Lock()
	defer tw.mu.Unlock()

	if !tw.running {
		return xerrors.ErrNotRunning
	}

	// 计算需要转多少圈。
	ticks := int(delay / tw.tick)
	circle := ticks / tw.wheelSize
	// 计算落在哪个槽位。
	index := (tw.current + ticks) % tw.wheelSize

	// 从池中获取节点。
	entry := tw.pool.Get().(*timerEntry)
	entry.expiration = time.Now().Add(delay)
	entry.circle = circle
	entry.task = task
	// 头插法插入链表 (O(1))。
	entry.next = tw.slots[index]
	tw.slots[index] = entry

	return nil
}

// tickTock 时间轮推进一格。
func (tw *TimingWheel) tickTock() {
	tw.mu.Lock()

	// 获取当前槽位的链表头引用。
	head := tw.slots[tw.current]
	var prev *timerEntry
	curr := head

	// 收集到期任务的链表，以便在锁外执行。
	var expiredHead *timerEntry
	var expiredTail *timerEntry

	// 遍历链表。
	for curr != nil {
		next := curr.next

		if curr.circle > 0 {
			// 未到期，圈数减一。
			curr.circle--
			prev = curr
			curr = next
		} else {
			// 到期了，从槽位链表中移除 curr。
			if prev == nil {
				tw.slots[tw.current] = next
			} else {
				prev.next = next
			}

			// 将 curr 添加到过期任务链表。
			curr.next = nil
			if expiredHead == nil {
				expiredHead = curr
				expiredTail = curr
			} else {
				expiredTail.next = curr
				expiredTail = curr
			}

			// 继续处理下一个。
			curr = next
		}
	}

	// 指针前进。
	tw.current = (tw.current + 1) % tw.wheelSize
	tw.mu.Unlock() // 尽早释放锁

	// 在锁外执行任务并回收节点。
	curr = expiredHead
	for curr != nil {
		next := curr.next

		// 执行任务 (异步，避免阻塞时间轮推进)。
		if curr.task != nil {
			go curr.task()
		}

		// 重置并放回池中。
		curr.task = nil
		curr.next = nil
		tw.pool.Put(curr)

		curr = next
	}
}
