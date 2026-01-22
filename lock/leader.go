package lock

import (
	"context"
	"log/slog"
	"time"
)

// LeaderElector 选举协调器
type LeaderElector struct {
	lock DistributedLock
	key  string
	ttl  time.Duration
}

func NewLeaderElector(lock DistributedLock, key string, ttl time.Duration) *LeaderElector {
	return &LeaderElector{
		lock: lock,
		key:  key,
		ttl:  ttl,
	}
}

// Campaign 参与竞选，成功则执行 callback，直到失去领导权或 ctx 结束
func (e *LeaderElector) Campaign(ctx context.Context, callback func(ctx context.Context)) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 尝试获取领导权 (带续期)
			token, stop, err := e.lock.LockWithWatchdog(ctx, e.key, e.ttl)
			if err != nil {
				// 获取失败，稍后重试
				time.Sleep(2 * time.Second)
				continue
			}

			slog.Info("Successfully elected as leader", "key", e.key)

			// 执行领导者逻辑
			callback(ctx)

			// 正常退出或 ctx 结束，释放锁
			stop()
			_ = e.lock.Unlock(context.WithoutCancel(ctx), e.key, token)
			slog.Info("Successfully resigned from leader", "key", e.key)
		}
	}
}
