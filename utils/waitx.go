// Package utils 提供了通用的并发同步工具.
package utils

import (
	"sync"

	"github.com/wyfcoding/pkg/async"
)

// WaitGroup 封装了 sync.WaitGroup，并在 Go 方法中自动集成了 panic 恢复.
type WaitGroup struct {
	wg sync.WaitGroup
}

// Add 增加计数.
func (w *WaitGroup) Add(delta int) {
	w.wg.Add(delta)
}

// Done 减少计数.
func (w *WaitGroup) Done() {
	w.wg.Done()
}

// Wait 等待完成.
func (w *WaitGroup) Wait() {
	w.wg.Wait()
}

// Go 启动一个带 panic 保护的协程，并自动管理计数.
func (w *WaitGroup) Go(fn func()) {
	w.wg.Add(1)
	async.SafeGo(func() {
		defer w.wg.Done()
		fn()
	})
}
