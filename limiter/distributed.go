// 变更说明：分布式限流器增强版，支持多实例共享限流状态、动态配置、多维度限流
package limiter

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// DistributedLimiterConfig 分布式限流器配置
type DistributedLimiterConfig struct {
	KeyPrefix       string        // Redis key 前缀
	DefaultRate     int           // 默认每秒请求数
	DefaultBurst    int           // 默认突发容量
	WindowSize      time.Duration // 滑动窗口大小
	MaxRetry        int           // 最大重试次数
	RetryInterval   time.Duration // 重试间隔
	FallbackToLocal bool          // Redis 失败时降级到本地限流
}

// DistributedLimiter 分布式限流器
type DistributedLimiter struct {
	client         redis.UniversalClient
	config         DistributedLimiterConfig
	localLimiters  sync.Map      // 本地限流器降级缓存
	dynamicConfigs sync.Map      // 动态配置缓存
	slidingScript  *redis.Script // 滑动窗口脚本
	tokenScript    *redis.Script // 令牌桶脚本
}

// NewDistributedLimiter 创建分布式限流器
func NewDistributedLimiter(client redis.UniversalClient, config DistributedLimiterConfig) *DistributedLimiter {
	if config.KeyPrefix == "" {
		config.KeyPrefix = "limiter"
	}
	if config.DefaultRate <= 0 {
		config.DefaultRate = 100
	}
	if config.DefaultBurst <= 0 {
		config.DefaultBurst = int(float64(config.DefaultRate) * 1.2)
	}
	if config.WindowSize <= 0 {
		config.WindowSize = time.Second
	}
	if config.MaxRetry <= 0 {
		config.MaxRetry = 3
	}
	if config.RetryInterval <= 0 {
		config.RetryInterval = 10 * time.Millisecond
	}

	dl := &DistributedLimiter{
		client:        client,
		config:        config,
		slidingScript: redis.NewScript(slidingWindowLua),
		tokenScript:   redis.NewScript(redisTokenBucketScript),
	}

	slog.Info("distributed_limiter initialized", "config", config)
	return dl
}

// Allow 检查是否允许通过
func (l *DistributedLimiter) Allow(ctx context.Context, key string) (bool, error) {
	return l.AllowWithRate(ctx, key, 0, 0)
}

// AllowWithRate 使用指定速率检查
func (l *DistributedLimiter) AllowWithRate(ctx context.Context, key string, reqRate, burst int) (bool, error) {
	if reqRate <= 0 {
		reqRate = l.config.DefaultRate
	}
	if burst <= 0 {
		burst = l.config.DefaultBurst
	}

	redisKey := fmt.Sprintf("%s:%s", l.config.KeyPrefix, key)

	var lastErr error
	for i := 0; i < l.config.MaxRetry; i++ {
		allowed, err := l.allowFromRedis(ctx, redisKey, reqRate, burst)
		if err == nil {
			return allowed, nil
		}
		lastErr = err
		slog.WarnContext(ctx, "redis limiter failed, retrying", "key", key, "attempt", i+1, "error", err)
		time.Sleep(l.config.RetryInterval)
	}

	if l.config.FallbackToLocal {
		slog.WarnContext(ctx, "falling back to local limiter", "key", key)
		return l.allowFromLocal(ctx, key, reqRate, burst)
	}

	return false, fmt.Errorf("distributed limiter failed after %d retries: %w", l.config.MaxRetry, lastErr)
}

// allowFromRedis 从 Redis 检查限流
func (l *DistributedLimiter) allowFromRedis(ctx context.Context, key string, reqRate, burst int) (bool, error) {
	now := time.Now().Unix()
	result, err := l.tokenScript.Run(ctx, l.client, []string{key}, reqRate, burst, now).Int()
	if err != nil {
		return false, fmt.Errorf("redis script run failed: %w", err)
	}
	return result == 1, nil
}

// allowFromLocal 从本地限流器检查（降级方案）
func (l *DistributedLimiter) allowFromLocal(ctx context.Context, key string, reqRate, burst int) (bool, error) {
	limiterKey := fmt.Sprintf("%s:%d:%d", key, reqRate, burst)
	if limiter, ok := l.localLimiters.Load(limiterKey); ok {
		return limiter.(*LocalLimiter).Allow(ctx, key)
	}

	newLimiter := NewLocalLimiter(rate.Limit(float64(reqRate)), burst)
	actual, _ := l.localLimiters.LoadOrStore(limiterKey, newLimiter)
	return actual.(*LocalLimiter).Allow(ctx, key)
}

// SlidingWindowAllow 滑动窗口限流
func (l *DistributedLimiter) SlidingWindowAllow(ctx context.Context, key string, limit int64) (bool, error) {
	now := time.Now()
	nowMs := now.UnixNano() / 1e6
	windowStartMs := now.Add(-l.config.WindowSize).UnixNano() / 1e6
	redisKey := fmt.Sprintf("%s:sw:%s", l.config.KeyPrefix, key)

	result, err := l.slidingScript.Run(ctx, l.client, []string{redisKey}, nowMs, windowStartMs, limit).Int()
	if err != nil {
		if l.config.FallbackToLocal {
			return l.allowFromLocal(ctx, key, int(limit), int(limit))
		}
		return false, err
	}

	return result == 1, nil
}

// MultiDimensionalLimiter 多维度限流器
type MultiDimensionalLimiter struct {
	distributed *DistributedLimiter
	limits      map[string]*DimensionLimit
}

// DimensionLimit 维度限流配置
type DimensionLimit struct {
	Key   string // 维度标识
	Rate  int    // 速率
	Burst int    // 突发容量
}

// NewMultiDimensionalLimiter 创建多维度限流器
func NewMultiDimensionalLimiter(distributed *DistributedLimiter, limits map[string]*DimensionLimit) *MultiDimensionalLimiter {
	return &MultiDimensionalLimiter{
		distributed: distributed,
		limits:      limits,
	}
}

// AllowAll 检查所有维度是否都允许通过
func (l *MultiDimensionalLimiter) AllowAll(ctx context.Context, keys map[string]string) (bool, error) {
	for dimension, value := range keys {
		limit, ok := l.limits[dimension]
		if !ok {
			continue
		}

		key := fmt.Sprintf("%s:%s", dimension, value)
		allowed, err := l.distributed.AllowWithRate(ctx, key, limit.Rate, limit.Burst)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

// AllowAny 检查是否有任一维度允许通过
func (l *MultiDimensionalLimiter) AllowAny(ctx context.Context, keys map[string]string) (bool, error) {
	for dimension, value := range keys {
		limit, ok := l.limits[dimension]
		if !ok {
			continue
		}

		key := fmt.Sprintf("%s:%s", dimension, value)
		allowed, err := l.distributed.AllowWithRate(ctx, key, limit.Rate, limit.Burst)
		if err != nil {
			return false, err
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}

// ClusterLimiter 集群限流器（多实例共享限流状态）
type ClusterLimiter struct {
	client        redis.UniversalClient
	clusterKey    string
	totalRate     int
	totalBurst    int
	instanceID    string
	syncInterval  time.Duration
	lastSync      time.Time
	localRate     int
	localBurst    int
	mu            sync.RWMutex
	localLimiters sync.Map
}

// NewClusterLimiter 创建集群限流器
func NewClusterLimiter(client redis.UniversalClient, clusterKey string, totalRate, totalBurst int, instanceID string) *ClusterLimiter {
	return &ClusterLimiter{
		client:     client,
		clusterKey: clusterKey,
		totalRate:  totalRate,
		totalBurst: totalBurst,
		instanceID: instanceID,
		localRate:  totalRate / 10,
		localBurst: totalBurst / 10,
	}
}

// Allow 检查是否允许通过
func (l *ClusterLimiter) Allow(ctx context.Context, key string) (bool, error) {
	l.mu.RLock()
	localRate := l.localRate
	localBurst := l.localBurst
	l.mu.RUnlock()

	limiterKey := fmt.Sprintf("%s:%d:%d", l.clusterKey, localRate, localBurst)
	if limiter, ok := l.localLimiters.Load(limiterKey); ok {
		return limiter.(*LocalLimiter).Allow(ctx, key)
	}

	newLimiter := NewLocalLimiter(rate.Limit(float64(localRate)), localBurst)
	actual, _ := l.localLimiters.LoadOrStore(limiterKey, newLimiter)
	return actual.(*LocalLimiter).Allow(ctx, key)
}

// SyncWithCluster 与集群同步限流配额
func (l *ClusterLimiter) SyncWithCluster(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if now.Sub(l.lastSync) < l.syncInterval {
		return nil
	}

	instanceCount, err := l.getClusterInstanceCount(ctx)
	if err != nil {
		return err
	}

	if instanceCount > 0 {
		l.localRate = l.totalRate / instanceCount
		l.localBurst = l.totalBurst / instanceCount
		if l.localRate < 1 {
			l.localRate = 1
		}
		if l.localBurst < 1 {
			l.localBurst = 1
		}
	}

	l.lastSync = now
	return nil
}

// getClusterInstanceCount 获取集群实例数量
func (l *ClusterLimiter) getClusterInstanceCount(ctx context.Context) (int, error) {
	key := fmt.Sprintf("%s:instances", l.clusterKey)
	now := time.Now().Unix()

	pipe := l.client.TxPipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", now-60))
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: l.instanceID})
	pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, 2*time.Minute)

	cmders, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}

	if len(cmders) >= 3 {
		return int(cmders[2].(*redis.IntCmd).Val()), nil
	}
	return 1, nil
}

// RateLimiterStats 限流器统计信息
type RateLimiterStats struct {
	Key          string    `json:"key"`
	AllowedCount int64     `json:"allowed_count"`
	DeniedCount  int64     `json:"denied_count"`
	CurrentRate  int       `json:"current_rate"`
	CurrentBurst int       `json:"current_burst"`
	LastUpdated  time.Time `json:"last_updated"`
}

// GetStats 获取限流器统计信息
func (l *DistributedLimiter) GetStats(ctx context.Context, key string) (*RateLimiterStats, error) {
	redisKey := fmt.Sprintf("%s:%s", l.config.KeyPrefix, key)
	data, err := l.client.HGetAll(ctx, redisKey).Result()
	if err != nil {
		return nil, err
	}

	stats := &RateLimiterStats{
		Key:          key,
		CurrentRate:  l.config.DefaultRate,
		CurrentBurst: l.config.DefaultBurst,
		LastUpdated:  time.Now(),
	}

	if v, ok := data["allowed_count"]; ok {
		fmt.Sscanf(v, "%d", &stats.AllowedCount)
	}
	if v, ok := data["denied_count"]; ok {
		fmt.Sscanf(v, "%d", &stats.DeniedCount)
	}
	if v, ok := data["current_rate"]; ok {
		fmt.Sscanf(v, "%d", &stats.CurrentRate)
	}
	if v, ok := data["current_burst"]; ok {
		fmt.Sscanf(v, "%d", &stats.CurrentBurst)
	}

	return stats, nil
}

// SetDynamicRate 动态设置限流速率
func (l *DistributedLimiter) SetDynamicRate(ctx context.Context, key string, rate, burst int) error {
	configKey := fmt.Sprintf("%s:config:%s", l.config.KeyPrefix, key)
	return l.client.HSet(ctx, configKey, map[string]any{
		"rate":  rate,
		"burst": burst,
	}).Err()
}

// GetDynamicRate 获取动态限流速率
func (l *DistributedLimiter) GetDynamicRate(ctx context.Context, key string) (int, int, error) {
	configKey := fmt.Sprintf("%s:config:%s", l.config.KeyPrefix, key)
	data, err := l.client.HGetAll(ctx, configKey).Result()
	if err != nil {
		return l.config.DefaultRate, l.config.DefaultBurst, nil
	}

	rate := l.config.DefaultRate
	burst := l.config.DefaultBurst

	if v, ok := data["rate"]; ok {
		fmt.Sscanf(v, "%d", &rate)
	}
	if v, ok := data["burst"]; ok {
		fmt.Sscanf(v, "%d", &burst)
	}

	return rate, burst, nil
}

// slidingWindowLua 滑动窗口 Lua 脚本
const slidingWindowLua = `
	local key = KEYS[1]
	local now = tonumber(ARGV[1])
	local start = tonumber(ARGV[2])
	local limit = tonumber(ARGV[3])
	
	redis.call('ZREMRANGEBYSCORE', key, 0, start)
	local count = redis.call('ZCARD', key)
	
	if count < limit then
		redis.call('ZADD', key, now, now .. '_' .. math.random())
		redis.call('PEXPIRE', key, (now - start) * 2)
		return 1
	else
		return 0
	end
`
