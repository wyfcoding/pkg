package algorithm

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"github.com/sourcegraph/conc"
)

// TimerTask 定义定时任务函数签名
type TimerTask func()

// timerEntry 内部任务包装
type timerEntry struct {
	expiration time.Time // 绝对过期时间，用于校对
	circle     int       // 剩余圈数
	task       TimerTask
}

// TimingWheel 是一个基于“圈数”的单层时间轮实现。
// 相比层级时间轮，它实现更简单，且在绝大多数业务场景下性能表现相当。
// 复杂度：插入 O(1)，推进 O(1)
type TimingWheel struct {
	tick      time.Duration // 每一格的时间跨度 (例如 1s)
	wheelSize int           // 轮子的槽位数量 (例如 3600)
	interval  time.Duration // 这一层转一圈的总时间 (tick * wheelSize)
	current   int           // 当前指针位置
	slots     []*list.List  // 槽位链表
	exitC     chan struct{}
	wg        conc.WaitGroup
	mu        sync.Mutex
	running   bool
}

// NewTimingWheel 创建一个新的时间轮
// tick: 时间粒度 (如 1s)
// wheelSize: 槽位数量 (如 3600)。对于 1s 一格，3600 格意味着一圈是 1 小时。
func NewTimingWheel(tick time.Duration, wheelSize int) (*TimingWheel, error) {
	if tick <= 0 || wheelSize <= 0 {
		return nil, errors.New("tick and wheelSize must be positive")
	}

	tw := &TimingWheel{
		tick:      tick,
		wheelSize: wheelSize,
		interval:  tick * time.Duration(wheelSize),
		slots:     make([]*list.List, wheelSize),
		exitC:     make(chan struct{}),
	}
	for i := range tw.slots {
		tw.slots[i] = list.New()
	}
	return tw, nil
}

// Start 启动时间轮
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

// Stop 停止时间轮
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

// AddTask 添加一个延迟任务
// delay: 延迟执行的时间
// task: 任务回调函数
func (tw *TimingWheel) AddTask(delay time.Duration, task TimerTask) error {
	if delay < 0 {
		return errors.New("delay must be non-negative")
	}

	tw.mu.Lock()
	defer tw.mu.Unlock()

	if !tw.running {
		return errors.New("timing wheel is not running")
	}

	// 计算需要转多少圈
	ticks := int(delay / tw.tick)
	circle := ticks / tw.wheelSize
	// 计算落在哪个槽位
	index := (tw.current + ticks) % tw.wheelSize

	tw.slots[index].PushBack(&timerEntry{
		expiration: time.Now().Add(delay),
		circle:     circle,
		task:       task,
	})
	return nil
}

// tickTock 时间轮推进一格
func (tw *TimingWheel) tickTock() {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	// 获取当前槽位的链表
	l := tw.slots[tw.current]

	// 遍历并处理任务
	var next *list.Element
	for e := l.Front(); e != nil; e = next {
		next = e.Next()
		entry := e.Value.(*timerEntry)

		if entry.circle > 0 {
			// 如果圈数大于0，说明还没到期，圈数减一
			entry.circle--
		} else {
			// 到期了，执行任务
			// 最好异步执行，避免阻塞时间轮推进
			go entry.task()
			l.Remove(e)
		}
	}

	// 指针前移
	tw.current = (tw.current + 1) % tw.wheelSize
}
