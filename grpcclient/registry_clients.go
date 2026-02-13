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
	"github.com/wyfcoding/pkg/registry"
	"google.golang.org/grpc"
)

var (
	// ErrRegistryNil 注册中心为空。
	ErrRegistryNil = errors.New("registry is nil")
)

// InitClientsWithRegistry 使用注册中心初始化 gRPC 客户端连接。
func InitClientsWithRegistry(reg registry.Registry, metricsInstance *metrics.Metrics, cbCfg config.CircuitBreakerConfig, targetClients any) (func(), error) {
	if reg == nil {
		return nil, ErrRegistryNil
	}

	RegisterRegistryResolver(reg, logging.Default())

	val := reflect.ValueOf(targetClients)
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return nil, ErrInvalidTarget
	}

	elem := val.Elem()
	typ := elem.Type()

	conns := make([]*grpc.ClientConn, 0, elem.NumField())
	factory := pickFactory(metricsInstance, cbCfg)

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
		if serviceName == "" {
			continue
		}

		target := RegistryTarget(serviceName)
		conn, err := factory.NewClient(target)
		if err != nil {
			for _, connItem := range conns {
				connItem.Close()
			}
			return nil, fmt.Errorf("%w %s: %w", ErrConnectService, serviceName, err)
		}

		conns = append(conns, conn)
		field.Set(reflect.ValueOf(conn))
		slog.Info("connected to service via registry", "service", serviceName, "target", target)
	}

	cleanup := func() {
		for _, c := range conns {
			c.Close()
		}
	}

	return cleanup, nil
}

// InitClientsAuto 自动选择注册中心或静态配置初始化 gRPC 客户端。
func InitClientsAuto(services map[string]config.ServiceAddr, metricsInstance *metrics.Metrics, cbCfg config.CircuitBreakerConfig, targetClients any) (func(), error) {
	if reg := registry.Default(); reg != nil {
		return InitClientsWithRegistry(reg, metricsInstance, cbCfg, targetClients)
	}
	return InitClients(services, metricsInstance, cbCfg, targetClients)
}
