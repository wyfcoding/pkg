// Package dtm 提供了 DTM 分布式事务的封装。
package dtm

import (
	"context"
	"fmt"
	"time"

	"github.com/dtm-labs/client/dtmgrpc"
	"github.com/wyfcoding/pkg/logging"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Tcc 封装了 DTM 的 Try-Confirm-Cancel 分布式事务模式，用于处理对一致性要求极高的跨服务资源操作。
type Tcc struct { //nolint:govet
	server string // DTM 服务端地址 (Host:Port)。
	gid    string // 全局唯一的事务标识。
}

// NewTcc 构造一个新的 TCC 事务控制器。
func NewTcc(server, gid string) *Tcc {
	return &Tcc{
		server: server,
		gid:    gid,
	}
}

// Execute 开启并执行整个 TCC 全局事务。
// 此方法会自动执行：初始化全局事务 -> 执行业务 Try 逻辑 -> 根据结果自动调用 Confirm 或 Cancel。
func (t *Tcc) Execute(ctx context.Context, handler func(*dtmgrpc.TccGrpc) error) error {
	logger := logging.Default()
	start := time.Now()
	logger.InfoContext(ctx, "executing tcc transaction", "gid", t.gid, "server", t.server)

	err := dtmgrpc.TccGlobalTransaction(t.server, t.gid, func(tcc *dtmgrpc.TccGrpc) error {
		return handler(tcc)
	})

	duration := time.Since(start)
	if err != nil {
		logger.ErrorContext(ctx, "tcc transaction failed", "gid", t.gid, "error", err, "duration", duration)

		return fmt.Errorf("tcc transaction execution error: %w", err)
	}

	logger.InfoContext(ctx, "tcc transaction finished successfully", "gid", t.gid, "duration", duration)

	return nil
}

// CallBranch 是对 DTM 子分支调用的标准化封装。
// 流程：在 gRPC 模式下注册 Try, Confirm, Cancel 三个阶段的远程服务路径。
func CallBranch(tcc *dtmgrpc.TccGrpc, payload proto.Message, try, confirm, cancel string) error {
	// 使用 emptypb.Empty 作为标准 gRPC 响应占位符，符合项目 API 规范。
	if err := tcc.CallBranch(payload, try, confirm, cancel, &emptypb.Empty{}); err != nil {
		return fmt.Errorf("dtm call branch failed: %w", err)
	}

	return nil
}
