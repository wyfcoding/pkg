package grpcclient

import (
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"google.golang.org/grpc"
)

var (
	// ErrInvalidTarget 目标客户端必须是结构体指针.
	ErrInvalidTarget = errors.New("targetClients must be a pointer to a struct")
	// ErrConnectService 连接服务失败.
	ErrConnectService = errors.New("failed to connect to service")
)

// InitClients 根据 ServiceClients 结构体字段和 Services 配置映射初始化 gRPC 客户端.
func InitClients(services map[string]config.ServiceAddr, metricsInstance *metrics.Metrics, cbCfg config.CircuitBreakerConfig, targetClients any) (func(), error) {
	val := reflect.ValueOf(targetClients)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return nil, ErrInvalidTarget
	}

	elem := val.Elem()
	typ := elem.Type()

	var conns []*grpc.ClientConn

	factory := NewClientFactory(logging.Default(), metricsInstance, cbCfg)

	for i := range elem.NumField() {
		field := elem.Field(i)
		fieldType := typ.Field(i)

		if !field.CanSet() {
			continue
		}

		if field.Type() != reflect.TypeFor[*grpc.ClientConn]() {
			continue
		}

		serviceName := fieldType.Tag.Get("service")
		if serviceName == "" {
			serviceName = strings.ToLower(fieldType.Name)
		}

		addrConfig, ok := services[serviceName]
		if !ok {
			slog.Info("service config not found for field, skipping auto-wiring", "field", fieldType.Name, "service", serviceName)

			continue
		}

		if addrConfig.GRPCAddr == "" {
			slog.Warn("service gRPC address is empty", "service", serviceName)

			continue
		}

		conn, err := factory.NewClient(addrConfig.GRPCAddr)
		if err != nil {
			for _, connItem := range conns {
				connItem.Close()
			}

			return nil, fmt.Errorf("%w %s at %s: %w", ErrConnectService, serviceName, addrConfig.GRPCAddr, err)
		}

		conns = append(conns, conn)
		field.Set(reflect.ValueOf(conn))
		slog.Info("connected to service", "service", serviceName, "addr", addrConfig.GRPCAddr)
	}

	cleanup := func() {
		for _, c := range conns {
			c.Close()
		}
	}

	return cleanup, nil
}
