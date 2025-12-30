package algorithm

import (
	"container/list"
	"sync"
	"time"
)

// TimerTask 定时任务函数
type TimerTask func()

// TimingWheel 分层时间轮实现
type TimingWheel struct {
	interval    time.Duration // 每个刻度的时间跨度
	wheelSize   int           // 轮子的槽位数量
	currentTime time.Time     // 当前时间指针
	buckets     []*list.List  // 槽位，每个槽位是一个双向链表
	mu          sync.Mutex

	ticking bool
	stop    chan struct{}
}

// NewTimingWheel 创建一个新的时间轮
func NewTimingWheel(interval time.Duration, wheelSize int) *TimingWheel {
	tw := &TimingWheel{
		interval:    interval,
		wheelSize:   wheelSize,
		currentTime: time.Now().Truncate(interval),
		buckets:     make([]*list.List, wheelSize),
		stop:        make(chan struct{}),
	}
	for i := range tw.buckets {
		tw.buckets[i] = list.New()
	}
	return tw
}

type timerEntry struct {
	expiration time.Time
	task       TimerTask
}

// AddTask 添加一个延迟任务
func (tw *TimingWheel) AddTask(delay time.Duration, task TimerTask) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	expiration := time.Now().Add(delay)
	// 计算槽位索引
	offset := int(delay / tw.interval)
	index := (offset + tw.getCurrentIndex()) % tw.wheelSize

	tw.buckets[index].PushBack(&timerEntry{
		expiration: expiration,
		task:       task,
	})
}

func (tw *TimingWheel) getCurrentIndex() int {
	return int(tw.currentTime.UnixNano()/int64(tw.interval)) % tw.wheelSize
}

// Start 启动时间轮转动
func (tw *TimingWheel) Start() {
	tw.mu.Lock()
	if tw.ticking {
		tw.mu.Unlock()
		return
	}
	tw.ticking = true
	tw.mu.Unlock()

	go func() {
		ticker := time.NewTicker(tw.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				tw.advanceClock()
			case <-tw.stop:
				return
			}
		}
	}()
}

func (tw *TimingWheel) advanceClock() {
	tw.mu.Lock()
	tw.currentTime = tw.currentTime.Add(tw.interval)
	index := tw.getCurrentIndex()
	bucket := tw.buckets[index]

	// 提取该槽位中所有已到期的任务
	var expiredTasks []TimerTask
	for e := bucket.Front(); e != nil; {
		next := e.Next()
		entry := e.Value.(*timerEntry)
		if entry.expiration.Before(tw.currentTime) || entry.expiration.Equal(tw.currentTime) {
			expiredTasks = append(expiredTasks, entry.task)
			bucket.Remove(e)
		}
		e = next
	}
	tw.mu.Unlock()

	// 并发执行到期任务，防止阻塞时间轮转动
	for _, task := range expiredTasks {
		go task()
	}
}

// Stop 停止时间轮
func (tw *TimingWheel) Stop() {
	close(tw.stop)
}
