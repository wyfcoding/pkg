package health

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// GRPCHealthServer 提供基于本地健康检查的 gRPC Health 实现。
type GRPCHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
	service  string
	checkers []Checker
}

// NewGRPCHealthServer 创建 gRPC Health 服务实例。
func NewGRPCHealthServer(service string, checkers []Checker) *GRPCHealthServer {
	return &GRPCHealthServer{
		service:  service,
		checkers: checkers,
	}
}

// RegisterGRPCHealthServer 注册 gRPC Health 服务。
func RegisterGRPCHealthServer(s *grpc.Server, service string, checkers []Checker) {
	grpc_health_v1.RegisterHealthServer(s, NewGRPCHealthServer(service, checkers))
}

// Check 执行健康检查并返回服务状态。
func (g *GRPCHealthServer) Check(_ context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	if req != nil && req.Service != "" && g.service != "" && req.Service != g.service {
		return &grpc_health_v1.HealthCheckResponse{
			Status: grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN,
		}, nil
	}

	for _, checker := range g.checkers {
		if err := checker(); err != nil {
			return &grpc_health_v1.HealthCheckResponse{
				Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
			}, nil
		}
	}

	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}

// List 返回可用服务的健康快照。
func (g *GRPCHealthServer) List(_ context.Context, _ *grpc_health_v1.HealthListRequest) (*grpc_health_v1.HealthListResponse, error) {
	response := &grpc_health_v1.HealthListResponse{
		Statuses: make(map[string]*grpc_health_v1.HealthCheckResponse),
	}

	status := grpc_health_v1.HealthCheckResponse_SERVING
	for _, checker := range g.checkers {
		if err := checker(); err != nil {
			status = grpc_health_v1.HealthCheckResponse_NOT_SERVING
			break
		}
	}

	if g.service != "" {
		response.Statuses[g.service] = &grpc_health_v1.HealthCheckResponse{Status: status}
	}
	response.Statuses[""] = &grpc_health_v1.HealthCheckResponse{Status: status}

	return response, nil
}

// Watch 暂不支持流式健康订阅，直接返回未实现。
func (g *GRPCHealthServer) Watch(_ *grpc_health_v1.HealthCheckRequest, _ grpc.ServerStreamingServer[grpc_health_v1.HealthCheckResponse]) error {
	return status.Error(codes.Unimplemented, "health watch not supported")
}
