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

// InitServiceClients initializes gRPC clients based on the ServiceClients struct fields
// and the Services configuration map.
// It returns a cleanup function that closes all established connections.
func InitServiceClients(services map[string]config.ServiceAddr, targetClients interface{}) (func(), error) {
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

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Only process fields of type *grpc.ClientConn
		if field.Type() != reflect.TypeOf((*grpc.ClientConn)(nil)) {
			continue
		}

		// Determine service name from tag or field name
		serviceName := fieldType.Tag.Get("service")
		if serviceName == "" {
			serviceName = strings.ToLower(fieldType.Name)
		}

		// Look up service address in config
		addrConfig, ok := services[serviceName]
		if !ok {
			// Warn but don't fail, maybe it's optional or configured elsewhere?
			// Actually, for now let's just log info and skip.
			slog.Info("Service config not found for field, skipping auto-wiring", "field", fieldType.Name, "service", serviceName)
			continue
		}

		if addrConfig.GRPCAddr == "" {
			slog.Warn("Service gRPC address is empty", "service", serviceName)
			continue
		}

		// Dial
		conn, err := grpc.NewClient(addrConfig.GRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			// Close already opened connections
			for _, c := range conns {
				c.Close()
			}
			return nil, fmt.Errorf("failed to connect to service %s at %s: %w", serviceName, addrConfig.GRPCAddr, err)
		}

		conns = append(conns, conn)
		field.Set(reflect.ValueOf(conn))
		slog.Info("Connected to service", "service", serviceName, "addr", addrConfig.GRPCAddr)
	}

	cleanup := func() {
		for _, c := range conns {
			c.Close()
		}
	}

	return cleanup, nil
}
