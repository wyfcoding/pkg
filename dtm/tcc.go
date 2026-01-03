package dtm

import (
	"context"
	"fmt"

	"github.com/dtm-labs/client/dtmgrpc"
	"github.com/wyfcoding/pkg/logging"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Tcc 事务封装
type Tcc struct {
	server string
	gid    string
	ctx    context.Context
}

// NewTcc 创建一个新的 TCC 事务
func NewTcc(ctx context.Context, server string, gid string) *Tcc {
	return &Tcc{
		server: server,
		gid:    gid,
		ctx:    ctx,
	}
}

// Execute 执行 TCC 事务
func (t *Tcc) Execute(fn func(*dtmgrpc.TccGrpc) error) error {
	logger := logging.Default()
	logger.InfoContext(t.ctx, "executing tcc transaction", "gid", t.gid, "server", t.server)

	// 使用 dtmgrpc 的 TccGlobalTransaction，它会自动创建并提交/回滚事务
	err := dtmgrpc.TccGlobalTransaction(t.server, t.gid, func(tcc *dtmgrpc.TccGrpc) error {
		return fn(tcc)
	})

	if err != nil {
		logger.ErrorContext(t.ctx, "tcc transaction failed", "gid", t.gid, "error", err)
		return fmt.Errorf("tcc transaction error: %w", err)
	}

	return nil
}

// CallBranch 是对 tcc.CallBranch 的简单包装
// 注意：在 gRPC 模式下，必须提供 reply 占位符
func CallBranch(tcc *dtmgrpc.TccGrpc, payload proto.Message, try, confirm, cancel string) error {
	return tcc.CallBranch(payload, try, confirm, cancel, &emptypb.Empty{})
}

