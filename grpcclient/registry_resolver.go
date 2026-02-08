package grpcclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/registry"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

const registryScheme = "registry"
const (
	tagModeAny = "any"
	tagModeAll = "all"
)

var (
	registryResolverOnce sync.Once
	defaultRegistry      atomic.Value
	defaultSelector      atomic.Value
)

// RegistrySelector 定义注册中心解析的筛选策略。
type RegistrySelector struct {
	PreferredZone string
	RequiredTags  []string
	TagMode       string
}

// SetRegistrySelector 设置解析器筛选策略。
func SetRegistrySelector(selector RegistrySelector) {
	normalized := normalizeSelector(selector)
	defaultSelector.Store(normalized)
}

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
		backoff:     time.Second,
		maxBackoff:  30 * time.Second,
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
	backoff     time.Duration
	maxBackoff  time.Duration
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
	for {
		err := r.registry.Watch(r.ctx, r.serviceName, func(instances []registry.ServiceInstance) {
			r.update(instances)
		})
		if err == nil || r.ctx.Err() != nil {
			return
		}
		r.logger.Error("registry watch failed, retrying", "service", r.serviceName, "error", err)
		sleepWithBackoff(r.ctx, r.nextBackoff())
	}
}

func (r *registryResolver) update(instances []registry.ServiceInstance) {
	instances = applySelector(instances, getSelector())
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
		if instance.Zone != "" {
			attrs = attrs.WithValue("zone", instance.Zone)
		}
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

func getSelector() RegistrySelector {
	selector, ok := defaultSelector.Load().(RegistrySelector)
	if !ok {
		return RegistrySelector{}
	}
	return selector
}

func normalizeSelector(selector RegistrySelector) RegistrySelector {
	selector.PreferredZone = strings.TrimSpace(selector.PreferredZone)
	selector.RequiredTags = normalizeTags(selector.RequiredTags)
	switch strings.ToLower(strings.TrimSpace(selector.TagMode)) {
	case tagModeAll:
		selector.TagMode = tagModeAll
	case tagModeAny:
		selector.TagMode = tagModeAny
	default:
		selector.TagMode = tagModeAny
	}
	return selector
}

func normalizeTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		t := strings.TrimSpace(tag)
		if t == "" {
			continue
		}
		result = append(result, t)
	}
	return result
}

func applySelector(instances []registry.ServiceInstance, selector RegistrySelector) []registry.ServiceInstance {
	if len(instances) == 0 {
		return instances
	}

	candidates := instances
	if selector.PreferredZone != "" {
		zoneMatches := filterByZone(candidates, selector.PreferredZone)
		if len(zoneMatches) > 0 {
			candidates = zoneMatches
		}
	}

	if len(selector.RequiredTags) == 0 {
		return candidates
	}

	tagMatches := filterByTags(candidates, selector.RequiredTags, selector.TagMode)
	if len(tagMatches) == 0 {
		return nil
	}
	return tagMatches
}

func filterByZone(instances []registry.ServiceInstance, zone string) []registry.ServiceInstance {
	result := make([]registry.ServiceInstance, 0, len(instances))
	for _, instance := range instances {
		if instance.Zone == zone {
			result = append(result, instance)
		}
	}
	return result
}

func filterByTags(instances []registry.ServiceInstance, tags []string, mode string) []registry.ServiceInstance {
	result := make([]registry.ServiceInstance, 0, len(instances))
	for _, instance := range instances {
		if matchTags(instance.Tags, tags, mode) {
			result = append(result, instance)
		}
	}
	return result
}

func matchTags(instanceTags []string, required []string, mode string) bool {
	if len(required) == 0 {
		return true
	}
	tagSet := make(map[string]struct{}, len(instanceTags))
	for _, tag := range instanceTags {
		tagSet[tag] = struct{}{}
	}

	if mode == tagModeAll {
		for _, tag := range required {
			if _, ok := tagSet[tag]; !ok {
				return false
			}
		}
		return true
	}

	for _, tag := range required {
		if _, ok := tagSet[tag]; ok {
			return true
		}
	}
	return false
}

func (r *registryResolver) nextBackoff() time.Duration {
	if r.backoff <= 0 {
		r.backoff = time.Second
	}
	value := r.backoff
	next := time.Duration(math.Min(float64(r.maxBackoff), float64(r.backoff)*2))
	r.backoff = next
	return value
}

func sleepWithBackoff(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	}
}
