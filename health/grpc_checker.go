package health

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
)

const defaultGRPCHealthTimeout = 2 * time.Second

// GRPCChecker 返回 gRPC 依赖健康检查函数。
// service 为空时表示检查服务整体健康状态。
func GRPCChecker(addr, service string, timeout time.Duration) Checker {
	return GRPCCheckerWithOptions(addr, service, timeout)
}

// GRPCCheckerWithOptions 返回可自定义 Dial 选项的 gRPC 健康检查函数。
func GRPCCheckerWithOptions(addr, service string, timeout time.Duration, opts ...grpc.DialOption) Checker {
	return func() error {
		if addr == "" {
			return errors.New("grpc health addr is empty")
		}
		if timeout <= 0 {
			timeout = defaultGRPCHealthTimeout
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		dialOpts := make([]grpc.DialOption, 0, len(opts)+2)
		dialOpts = append(dialOpts, grpc.WithBlock())
		if len(opts) == 0 {
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
		dialOpts = append(dialOpts, opts...)

		conn, err := grpc.DialContext(ctx, addr, dialOpts...)
		if err != nil {
			return fmt.Errorf("grpc dial failed: %w", err)
		}
		defer conn.Close()

		client := grpc_health_v1.NewHealthClient(conn)
		resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
		if err != nil {
			return fmt.Errorf("grpc health check failed: %w", err)
		}
		if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			return fmt.Errorf("grpc health status: %s", resp.GetStatus().String())
		}

		return nil
	}
}
