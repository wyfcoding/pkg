// Package dtm 提供了 DTM 分布式事务的封装.
package dtm

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/dtm-labs/client/dtmgrpc"
	"github.com/wyfcoding/pkg/logging"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
)

// Saga 封装了 DTM 的 Saga 分布式事务模式，增强了追踪信息自动注入功能.
type Saga struct { // SAGA 事务结构，已对齐。
	saga   *dtmgrpc.SagaGrpc // 底层 DTM gRPC 客户端实例
	server string            // DTM 服务器地址 (Host:Port)
	gid    string            // 全局事务 ID (Global ID)
}

// NewSaga 创建一个新的 Saga 事务实例.
func NewSaga(ctx context.Context, server, gid string) *Saga {
	saga := dtmgrpc.NewSagaGrpc(server, gid)

	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		saga.CustomData = fmt.Sprintf(`{"trace_id":%s}`, strconv.Quote(spanCtx.TraceID().String()))
	}

	return &Saga{
		server: server,
		gid:    gid,
		saga:   saga,
	}
}

// Add 增加一个事务步骤.
func (s *Saga) Add(action, compensate string, payload proto.Message) *Saga {
	s.saga.Add(action, compensate, payload)

	return s
}

// Submit 提交 Saga 事务.
func (s *Saga) Submit(ctx context.Context) error {
	logger := logging.Default()
	logger.InfoContext(ctx, "submitting saga transaction", "gid", s.gid, "server", s.server)

	if err := s.saga.Submit(); err != nil {
		logger.ErrorContext(ctx, "saga submission failed", "gid", s.gid, "error", err)

		return fmt.Errorf("saga submit error: %w", err)
	}

	return nil
}

// Barrier 包装数据库操作以支持 DTM 子事务屏障.
func Barrier(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	barrier, err := dtmgrpc.BarrierFromGrpc(ctx)
	if err != nil {
		return fmt.Errorf("failed to get barrier from grpc context: %w", err)
	}

	if err := barrier.CallWithDB(db, fn); err != nil {
		return fmt.Errorf("dtm barrier call with db failed: %w", err)
	}

	return nil
}
