package idgen

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrSegmentNotInitialized = errors.New("segment generator not initialized")
	ErrSegmentExhausted      = errors.New("segment exhausted")
)

type SegmentConfig struct {
	KeyPrefix    string
	BusinessTag  string
	Step         int64
	MaxRetries   int
	RetryDelay   time.Duration
	PreloadRatio float64
}

type Segment struct {
	Start int64
	End   int64
	Value atomic.Int64
}

func (s *Segment) Next() (int64, bool) {
	for {
		current := s.Value.Load()
		if current >= s.End {
			return 0, false
		}
		if s.Value.CompareAndSwap(current, current+1) {
			return current + 1, true
		}
	}
}

func (s *Segment) Remaining() int64 {
	return s.End - s.Value.Load()
}

func (s *Segment) Used() int64 {
	return s.Value.Load() - s.Start
}

func (s *Segment) PercentUsed() float64 {
	total := s.End - s.Start
	if total == 0 {
		return 100
	}
	return float64(s.Used()) / float64(total) * 100
}

type SegmentGenerator struct {
	client     redis.UniversalClient
	config     SegmentConfig
	segments   [2]*Segment
	index      atomic.Int32
	mu         sync.Mutex
	loading    atomic.Bool
	initialized atomic.Bool
	logger     *slog.Logger
}

func NewSegmentGenerator(client redis.UniversalClient, config SegmentConfig, logger *slog.Logger) *SegmentGenerator {
	if config.KeyPrefix == "" {
		config.KeyPrefix = "segment"
	}
	if config.Step <= 0 {
		config.Step = 1000
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 10 * time.Millisecond
	}
	if config.PreloadRatio <= 0 {
		config.PreloadRatio = 0.2
	}

	return &SegmentGenerator{
		client: client,
		config: config,
		logger: logger,
	}
}

func (g *SegmentGenerator) Init(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	segment, err := g.loadSegment(ctx)
	if err != nil {
		return fmt.Errorf("failed to load initial segment: %w", err)
	}

	g.segments[0] = segment
	g.index.Store(0)
	g.initialized.Store(true)

	g.logger.Info("segment generator initialized",
		"business_tag", g.config.BusinessTag,
		"start", segment.Start,
		"end", segment.End,
	)

	return nil
}

func (g *SegmentGenerator) Generate(ctx context.Context) (int64, error) {
	if !g.initialized.Load() {
		return 0, ErrSegmentNotInitialized
	}

	currentIndex := g.index.Load()
	currentSegment := g.segments[currentIndex]

	if currentSegment == nil {
		g.mu.Lock()
		if err := g.loadNextSegmentIfNeeded(ctx); err != nil {
			g.mu.Unlock()
			return 0, err
		}
		currentIndex = g.index.Load()
		currentSegment = g.segments[currentIndex]
		g.mu.Unlock()
	}

	id, ok := currentSegment.Next()
	if !ok {
		g.mu.Lock()
		if err := g.switchSegment(ctx); err != nil {
			g.mu.Unlock()
			return 0, err
		}
		currentIndex = g.index.Load()
		currentSegment = g.segments[currentIndex]
		g.mu.Unlock()

		id, ok = currentSegment.Next()
		if !ok {
			return 0, ErrSegmentExhausted
		}
	}

	if currentSegment.PercentUsed() > (1-g.config.PreloadRatio)*100 {
		g.preloadSegment(ctx)
	}

	return id, nil
}

func (g *SegmentGenerator) switchSegment(ctx context.Context) error {
	nextIndex := (g.index.Load() + 1) % 2

	if g.segments[nextIndex] == nil {
		segment, err := g.loadSegment(ctx)
		if err != nil {
			return err
		}
		g.segments[nextIndex] = segment
	}

	g.segments[g.index.Load()] = nil
	g.index.Store(nextIndex)

	g.logger.Info("switched to new segment",
		"business_tag", g.config.BusinessTag,
		"segment_index", nextIndex,
		"start", g.segments[nextIndex].Start,
		"end", g.segments[nextIndex].End,
	)

	return nil
}

func (g *SegmentGenerator) loadNextSegmentIfNeeded(ctx context.Context) error {
	nextIndex := (g.index.Load() + 1) % 2
	if g.segments[nextIndex] != nil {
		return nil
	}

	segment, err := g.loadSegment(ctx)
	if err != nil {
		return err
	}
	g.segments[nextIndex] = segment

	return nil
}

func (g *SegmentGenerator) preloadSegment(ctx context.Context) {
	if g.loading.Swap(true) {
		return
	}

	go func() {
		defer g.loading.Store(false)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		g.mu.Lock()
		defer g.mu.Unlock()

		if err := g.loadNextSegmentIfNeeded(ctx); err != nil {
			g.logger.Error("failed to preload segment", "error", err)
		}
	}()
}

func (g *SegmentGenerator) loadSegment(ctx context.Context) (*Segment, error) {
	key := fmt.Sprintf("%s:%s", g.config.KeyPrefix, g.config.BusinessTag)

	script := `
		local key = KEYS[1]
		local step = tonumber(ARGV[1])
		
		local current = redis.call('GET', key)
		if current == false then
			current = 0
		else
			current = tonumber(current)
		end
		
		local new_value = current + step
		redis.call('SET', key, new_value)
		
		return {current + 1, new_value}
	`

	var result []int64
	var err error

	for i := 0; i < g.config.MaxRetries; i++ {
		res, err := g.client.Eval(ctx, script, []string{key}, g.config.Step).Result()
		if err == nil {
			if vals, ok := res.([]interface{}); ok && len(vals) == 2 {
				if start, ok := vals[0].(int64); ok {
					if end, ok := vals[1].(int64); ok {
						result = []int64{start, end}
						break
					}
				}
			}
		}

		g.logger.Warn("failed to load segment, retrying",
			"attempt", i+1,
			"error", err,
		)
		time.Sleep(g.config.RetryDelay)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load segment after %d retries: %w", g.config.MaxRetries, err)
	}

	segment := &Segment{
		Start: result[0],
		End:   result[1],
	}
	segment.Value.Store(result[0] - 1)

	return segment, nil
}

func (g *SegmentGenerator) GetStats() map[string]interface{} {
	currentIndex := g.index.Load()
	currentSegment := g.segments[currentIndex]

	stats := map[string]interface{}{
		"business_tag":  g.config.BusinessTag,
		"current_index": currentIndex,
		"initialized":   g.initialized.Load(),
		"loading":       g.loading.Load(),
	}

	if currentSegment != nil {
		stats["current_start"] = currentSegment.Start
		stats["current_end"] = currentSegment.End
		stats["current_value"] = currentSegment.Value.Load()
		stats["remaining"] = currentSegment.Remaining()
		stats["percent_used"] = currentSegment.PercentUsed()
	}

	nextIndex := (currentIndex + 1) % 2
	nextSegment := g.segments[nextIndex]
	stats["next_segment_loaded"] = nextSegment != nil

	return stats
}

type SegmentPool struct {
	generators sync.Map
	client     redis.UniversalClient
	config     SegmentConfig
	logger     *slog.Logger
}

func NewSegmentPool(client redis.UniversalClient, config SegmentConfig, logger *slog.Logger) *SegmentPool {
	return &SegmentPool{
		client: client,
		config: config,
		logger: logger,
	}
}

func (p *SegmentPool) GetGenerator(businessTag string) (*SegmentGenerator, error) {
	if gen, ok := p.generators.Load(businessTag); ok {
		return gen.(*SegmentGenerator), nil
	}

	config := p.config
	config.BusinessTag = businessTag

	gen := NewSegmentGenerator(p.client, config, p.logger)
	if err := gen.Init(context.Background()); err != nil {
		return nil, err
	}

	actual, _ := p.generators.LoadOrStore(businessTag, gen)
	return actual.(*SegmentGenerator), nil
}

func (p *SegmentPool) Generate(ctx context.Context, businessTag string) (int64, error) {
	gen, err := p.GetGenerator(businessTag)
	if err != nil {
		return 0, err
	}
	return gen.Generate(ctx)
}

func (p *SegmentPool) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	p.generators.Range(func(key, value interface{}) bool {
		tag := key.(string)
		gen := value.(*SegmentGenerator)
		stats[tag] = gen.GetStats()
		return true
	})
	return stats
}
