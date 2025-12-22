package grpcclient

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/wyfcoding/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// InitServiceClients 根据 ServiceClients 结构体字段和 Services 配置映射初始化 gRPC 客户端。
// 它返回一个清理函数，用于关闭所有建立的连接。
func InitServiceClients(services map[string]config.ServiceAddr, targetClients any) (func(), error) {
	val := reflect.ValueOf(targetClients)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("targetClients must be a pointer to a struct")
	}

	elem := val.Elem()
	typ := elem.Type()

	var conns []*grpc.ClientConn

	for i := 0; i < elem.NumField(); i++ {
		field := elem.Field(i)
		fieldType := typ.Field(i)

		// 跳过未导出的字段
		if !field.CanSet() {
			continue
		}

		// 仅处理 *grpc.ClientConn 类型的字段
		if field.Type() != reflect.TypeFor[*grpc.ClientConn]() {
			continue
		}

		// 从 tag 或字段名确定服务名称
		serviceName := fieldType.Tag.Get("service")
		if serviceName == "" {
			serviceName = strings.ToLower(fieldType.Name)
		}

		// 在配置中查找服务地址
		addrConfig, ok := services[serviceName]
		if !ok {
			// 警告但不失败，也许它是可选的或在其他地方配置？
			// 目前我们只记录信息并跳过。
			slog.Info("service config not found for field, skipping auto-wiring", "field", fieldType.Name, "service", serviceName)
			continue
		}

		if addrConfig.GRPCAddr == "" {
			slog.Warn("service gRPC address is empty", "service", serviceName)
			continue
		}

		// 拨号连接
		conn, err := grpc.NewClient(addrConfig.GRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			// 关闭已打开的连接
			for _, c := range conns {
				c.Close()
			}
			return nil, fmt.Errorf("failed to connect to service %s at %s: %w", serviceName, addrConfig.GRPCAddr, err)
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
