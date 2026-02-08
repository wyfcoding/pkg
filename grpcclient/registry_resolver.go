package grpcclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/registry"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

const registryScheme = "registry"

var (
	registryResolverOnce sync.Once
	defaultRegistry      atomic.Value
)

// RegisterRegistryResolver 注册基于注册中心的 gRPC 名称解析器。
// 注意：建议在应用启动阶段调用。
func RegisterRegistryResolver(reg registry.Registry, logger *logging.Logger) {
	if reg != nil {
		defaultRegistry.Store(reg)
	}
	registryResolverOnce.Do(func() {
		if logger == nil {
			logger = logging.Default()
		}
		resolver.Register(&registryResolverBuilder{logger: logger})
	})
}

// RegistryTarget 构造 registry scheme 的 gRPC 目标。
func RegistryTarget(serviceName string) string {
	return fmt.Sprintf("%s:///%s", registryScheme, serviceName)
}

type registryResolverBuilder struct {
	logger *logging.Logger
}

func (b *registryResolverBuilder) Scheme() string {
	return registryScheme
}

func (b *registryResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	serviceName := target.Endpoint()
	if serviceName == "" {
		return nil, errors.New("registry resolver service name is empty")
	}

	reg, ok := defaultRegistry.Load().(registry.Registry)
	if !ok || reg == nil {
		return nil, errors.New("registry resolver not configured")
	}

	r := &registryResolver{
		registry:    reg,
		serviceName: serviceName,
		cc:          cc,
		logger:      b.logger,
	}
	r.ctx, r.cancel = context.WithCancel(context.Background())

	if err := r.resolve(); err != nil {
		b.logger.Warn("registry resolver initial resolve failed", "service", serviceName, "error", err)
	}

	go r.watch()

	return r, nil
}

type registryResolver struct {
	registry    registry.Registry
	serviceName string
	cc          resolver.ClientConn
	logger      *logging.Logger
	ctx         context.Context
	cancel      context.CancelFunc
}

func (r *registryResolver) ResolveNow(_ resolver.ResolveNowOptions) {
	if err := r.resolve(); err != nil {
		r.logger.Warn("registry resolver resolve failed", "service", r.serviceName, "error", err)
	}
}

func (r *registryResolver) Close() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *registryResolver) resolve() error {
	instances, err := r.registry.GetService(r.ctx, r.serviceName)
	if err != nil {
		return err
	}
	r.update(instances)
	return nil
}

func (r *registryResolver) watch() {
	err := r.registry.Watch(r.ctx, r.serviceName, func(instances []registry.ServiceInstance) {
		r.update(instances)
	})
	if err != nil && r.ctx.Err() == nil {
		r.logger.Error("registry watch failed", "service", r.serviceName, "error", err)
	}
}

func (r *registryResolver) update(instances []registry.ServiceInstance) {
	addresses := make([]resolver.Address, 0, len(instances))
	seen := make(map[string]struct{})
	for _, instance := range instances {
		if instance.Address == "" {
			continue
		}
		if _, ok := seen[instance.Address]; ok {
			continue
		}
		seen[instance.Address] = struct{}{}
		attrs := attributes.New("id", instance.ID)
		attrs = attrs.WithValue("weight", instance.Weight)
		if len(instance.Tags) > 0 {
			attrs = attrs.WithValue("tags", strings.Join(instance.Tags, ","))
		}
		if len(instance.Metadata) > 0 {
			if metaJSON, err := json.Marshal(instance.Metadata); err == nil {
				attrs = attrs.WithValue("metadata", string(metaJSON))
			}
		}
		addresses = append(addresses, resolver.Address{
			Addr:       instance.Address,
			Attributes: attrs,
		})
	}

	_ = r.cc.UpdateState(resolver.State{Addresses: addresses})
	if len(addresses) == 0 {
		r.cc.ReportError(errors.New("no available instances"))
	}
}
