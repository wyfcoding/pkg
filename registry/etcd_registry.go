package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	defaultPrefix = "/services"
	defaultTTL    = 10 * time.Second
)

// Option 定义 EtcdRegistry 构造参数。
type Option func(*EtcdRegistry)

// WithPrefix 设置注册中心 key 前缀。
func WithPrefix(prefix string) Option {
	return func(r *EtcdRegistry) {
		if prefix != "" {
			r.prefix = normalizePrefix(prefix)
		}
	}
}

// WithTTL 设置服务实例续租 TTL。
func WithTTL(ttl time.Duration) Option {
	return func(r *EtcdRegistry) {
		if ttl > 0 {
			r.ttl = ttl
		}
	}
}

// WithMetrics 注入指标采集器。
func WithMetrics(m *metrics.Metrics) Option {
	return func(r *EtcdRegistry) {
		if m == nil {
			return
		}
		r.metrics = &registryMetrics{
			registerTotal: m.NewCounterVec(&prometheus.CounterOpts{
				Namespace: "pkg",
				Subsystem: "registry",
				Name:      "register_total",
				Help:      "Total number of registry register attempts",
			}, []string{"service", "status"}),
			deregisterTotal: m.NewCounterVec(&prometheus.CounterOpts{
				Namespace: "pkg",
				Subsystem: "registry",
				Name:      "deregister_total",
				Help:      "Total number of registry deregister attempts",
			}, []string{"service", "status"}),
			watchEvents: m.NewCounterVec(&prometheus.CounterOpts{
				Namespace: "pkg",
				Subsystem: "registry",
				Name:      "watch_events_total",
				Help:      "Total number of registry watch events",
			}, []string{"service"}),
			instances: m.NewGaugeVec(&prometheus.GaugeOpts{
				Namespace: "pkg",
				Subsystem: "registry",
				Name:      "instances",
				Help:      "Number of service instances",
			}, []string{"service"}),
		}
	}
}

type registryMetrics struct {
	registerTotal   *prometheus.CounterVec
	deregisterTotal *prometheus.CounterVec
	watchEvents     *prometheus.CounterVec
	instances       *prometheus.GaugeVec
}

// EtcdRegistry 基于 Etcd 的服务注册中心实现。
type EtcdRegistry struct {
	client  *clientv3.Client
	logger  *logging.Logger
	prefix  string
	ttl     time.Duration
	metrics *registryMetrics
}

// NewEtcdRegistry 创建 Etcd 注册中心。
func NewEtcdRegistry(client *clientv3.Client, logger *logging.Logger, opts ...Option) (*EtcdRegistry, error) {
	if client == nil {
		return nil, errors.New("etcd client is nil")
	}
	if logger == nil {
		logger = logging.Default()
	}

	registry := &EtcdRegistry{
		client: client,
		logger: logger,
		prefix: defaultPrefix,
		ttl:    defaultTTL,
	}

	for _, opt := range opts {
		opt(registry)
	}

	registry.prefix = normalizePrefix(registry.prefix)

	return registry, nil
}

// Register 注册服务实例并启动续租。
func (r *EtcdRegistry) Register(ctx context.Context, instance ServiceInstance) (func(), error) {
	if err := validateInstance(instance); err != nil {
		r.recordRegister(instance.Name, "failed")
		return nil, err
	}

	leaseResp, err := r.client.Grant(ctx, int64(r.ttl.Seconds()))
	if err != nil {
		r.recordRegister(instance.Name, "failed")
		return nil, fmt.Errorf("grant lease failed: %w", err)
	}

	instance.UpdatedAt = time.Now()
	value, err := json.Marshal(instance)
	if err != nil {
		r.recordRegister(instance.Name, "failed")
		return nil, fmt.Errorf("marshal instance failed: %w", err)
	}

	key := r.buildKey(instance.Name, instance.ID)
	if _, err := r.client.Put(ctx, key, string(value), clientv3.WithLease(leaseResp.ID)); err != nil {
		r.recordRegister(instance.Name, "failed")
		return nil, fmt.Errorf("register instance failed: %w", err)
	}

	keepAliveCtx, cancel := context.WithCancel(ctx)
	keepAliveCh, err := r.client.KeepAlive(keepAliveCtx, leaseResp.ID)
	if err != nil {
		cancel()
		r.recordRegister(instance.Name, "failed")
		return nil, fmt.Errorf("keepalive failed: %w", err)
	}

	go r.consumeKeepAlive(keepAliveCtx, keepAliveCh, instance.Name, instance.ID)

	r.recordRegister(instance.Name, "success")

	cleanup := func() {
		cancel()
		revokeCtx, revokeCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer revokeCancel()
		if _, err := r.client.Revoke(revokeCtx, leaseResp.ID); err != nil {
			r.logger.Error("failed to revoke lease", "service", instance.Name, "error", err)
		}
	}

	return cleanup, nil
}

// Deregister 注销服务实例。
func (r *EtcdRegistry) Deregister(ctx context.Context, instance ServiceInstance) error {
	if err := validateInstance(instance); err != nil {
		r.recordDeregister(instance.Name, "failed")
		return err
	}

	key := r.buildKey(instance.Name, instance.ID)
	if _, err := r.client.Delete(ctx, key); err != nil {
		r.recordDeregister(instance.Name, "failed")
		return fmt.Errorf("deregister instance failed: %w", err)
	}

	r.recordDeregister(instance.Name, "success")
	return nil
}

// GetService 获取指定服务的实例列表。
func (r *EtcdRegistry) GetService(ctx context.Context, serviceName string) ([]ServiceInstance, error) {
	if strings.TrimSpace(serviceName) == "" {
		return nil, ErrInvalidInstance
	}

	prefix := r.buildPrefix(serviceName)
	resp, err := r.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("get service failed: %w", err)
	}

	instances := make([]ServiceInstance, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var instance ServiceInstance
		if err := json.Unmarshal(kv.Value, &instance); err != nil {
			r.logger.Error("failed to unmarshal instance", "error", err, "key", string(kv.Key))
			continue
		}
		instances = append(instances, instance)
	}

	if r.metrics != nil && r.metrics.instances != nil {
		r.metrics.instances.WithLabelValues(serviceName).Set(float64(len(instances)))
	}

	return instances, nil
}

// Watch 监听服务实例变化。
func (r *EtcdRegistry) Watch(ctx context.Context, serviceName string, handler WatchHandler) error {
	if strings.TrimSpace(serviceName) == "" {
		return ErrInvalidInstance
	}
	if handler == nil {
		return ErrHandlerNil
	}

	prefix := r.buildPrefix(serviceName)
	watchChan := r.client.Watch(ctx, prefix, clientv3.WithPrefix())
	for {
		select {
		case resp, ok := <-watchChan:
			if !ok {
				return nil
			}
			if err := resp.Err(); err != nil {
				r.logger.Error("registry watch error", "service", serviceName, "error", err)
				continue
			}
			if r.metrics != nil && r.metrics.watchEvents != nil {
				r.metrics.watchEvents.WithLabelValues(serviceName).Inc()
			}
			instances, err := r.GetService(ctx, serviceName)
			if err != nil {
				r.logger.Error("registry fetch instances failed", "service", serviceName, "error", err)
				continue
			}
			handler(instances)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (r *EtcdRegistry) consumeKeepAlive(ctx context.Context, ch <-chan *clientv3.LeaseKeepAliveResponse, serviceName, instanceID string) {
	for {
		select {
		case resp, ok := <-ch:
			if !ok {
				r.logger.Warn("etcd keepalive channel closed", "service", serviceName, "instance", instanceID)
				return
			}
			if resp == nil {
				continue
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *EtcdRegistry) buildPrefix(serviceName string) string {
	return fmt.Sprintf("%s/%s/", r.prefix, serviceName)
}

func (r *EtcdRegistry) buildKey(serviceName, instanceID string) string {
	return fmt.Sprintf("%s/%s/%s", r.prefix, serviceName, instanceID)
}

func validateInstance(instance ServiceInstance) error {
	if strings.TrimSpace(instance.Name) == "" || strings.TrimSpace(instance.ID) == "" || strings.TrimSpace(instance.Address) == "" {
		return ErrInvalidInstance
	}
	return nil
}

func normalizePrefix(prefix string) string {
	if prefix == "" {
		return defaultPrefix
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimRight(prefix, "/")
}

func (r *EtcdRegistry) recordRegister(serviceName, status string) {
	if r.metrics != nil && r.metrics.registerTotal != nil && serviceName != "" {
		r.metrics.registerTotal.WithLabelValues(serviceName, status).Inc()
	}
}

func (r *EtcdRegistry) recordDeregister(serviceName, status string) {
	if r.metrics != nil && r.metrics.deregisterTotal != nil && serviceName != "" {
		r.metrics.deregisterTotal.WithLabelValues(serviceName, status).Inc()
	}
}
