package clickhouse

import (
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/wyfcoding/pkg/config"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type dynamicState struct {
	conn driver.Conn
}

// DynamicConn 提供支持热更新的 ClickHouse 连接包装器。
type DynamicConn struct {
	mu    sync.Mutex
	state atomic.Value
}

// NewDynamicConn 创建支持热更新的 ClickHouse 连接。
func NewDynamicConn(cfg config.ClickHouseConfig) (*DynamicConn, error) {
	conn, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	d := &DynamicConn{}
	d.state.Store(&dynamicState{conn: conn})
	return d, nil
}

// UpdateConfig 使用最新配置刷新 ClickHouse 连接。
func (d *DynamicConn) UpdateConfig(cfg config.ClickHouseConfig) error {
	if d == nil {
		return errors.New("dynamic clickhouse client is nil")
	}
	conn, err := NewClient(cfg)
	if err != nil {
		return err
	}

	d.mu.Lock()
	old := d.load()
	d.state.Store(&dynamicState{conn: conn})
	d.mu.Unlock()

	if old != nil {
		if closer, ok := old.conn.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				slog.Error("failed to close old clickhouse connection", "error", err)
			}
		}
	}

	slog.Info("clickhouse client updated", "addr", cfg.Addr, "db", cfg.Database)

	return nil
}

// Conn 返回当前 ClickHouse 连接实例。
func (d *DynamicConn) Conn() driver.Conn {
	state := d.load()
	if state == nil {
		return nil
	}
	return state.conn
}

// Close 关闭当前 ClickHouse 连接。
func (d *DynamicConn) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	old := d.load()
	d.state.Store((*dynamicState)(nil))
	d.mu.Unlock()

	if old != nil {
		if closer, ok := old.conn.(interface{ Close() error }); ok {
			return closer.Close()
		}
	}

	return nil
}

// RegisterReloadHook 注册 ClickHouse 客户端热更新回调。
func RegisterReloadHook(conn *DynamicConn) {
	if conn == nil {
		return
	}
	config.RegisterReloadHook(func(updated *config.Config) {
		if updated == nil {
			return
		}
		if err := conn.UpdateConfig(updated.Data.ClickHouse); err != nil {
			slog.Error("clickhouse client reload failed", "error", err)
		}
	})
}

func (d *DynamicConn) load() *dynamicState {
	if d == nil {
		return nil
	}
	value := d.state.Load()
	if value == nil {
		return nil
	}
	return value.(*dynamicState)
}
