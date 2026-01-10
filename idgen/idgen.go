// Package idgen 提供了全局唯一的 ID 生成能力，支持 Sonyflake 与基础雪花算法.
package idgen

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sony/sonyflake"
)

var (
	// ErrCreateSonyflake 无法创建 Sonyflake 实例.
	ErrCreateSonyflake = errors.New("failed to create sonyflake")
	// ErrInvalidMachineID 机器 ID 超出有效范围.
	ErrInvalidMachineID = errors.New("invalid machine id")
	// ErrGeneratorFailed 生成器重试失败.
	ErrGeneratorFailed = errors.New("id generator failed after retries")
)

// Generator 定义了 ID 生成器的标准接口.
type Generator interface {
	NextID() (int64, error)
}

// Config ID 生成器的配置参数.
type Config struct {
	StartTime time.Time `mapstructure:"start_time" toml:"start_time"`
	MachineID int64     `mapstructure:"machine_id" toml:"machine_id"`
}

// SonyGenerator 基于 Sonyflake 实现的生成器.
type SonyGenerator struct {
	sf *sonyflake.Sonyflake
}

// NewSonyGenerator 构造一个新的 Sonyflake 生成器.
func NewSonyGenerator(cfg *Config) (*SonyGenerator, error) {
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	if cfg.MachineID < 0 || cfg.MachineID > 65535 {
		return nil, ErrInvalidMachineID
	}

	// 预计算 machineID 以避免在闭包中进行类型转换警告。
	// 安全：MachineID 已在上方验证范围 [0, 65535]。
	machineID := uint16(cfg.MachineID & 0xFFFF) //nolint:gosec // MachineID 已验证范围。

	settings := sonyflake.Settings{
		StartTime: startTime,
		MachineID: func() (uint16, error) {
			return machineID, nil
		},
	}

	sonyFlake, err := sonyflake.New(settings)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateSonyflake, err)
	}

	slog.Info("sonyflake generator initialized", "machine_id", cfg.MachineID, "start_time", startTime)

	return &SonyGenerator{sf: sonyFlake}, nil
}

// NextID 生成下一个分布式唯一 ID.
func (g *SonyGenerator) NextID() (int64, error) {
	const maxRetries = 3
	for i := range maxRetries {
		id, err := g.sf.NextID()
		if err == nil {
			// 使用位掩码确保最高位为 0，产出正数 int64。
			// 此转换是安全的：掩码后的值 <= math.MaxInt64。
			masked := id & 0x7FFFFFFFFFFFFFFF
			return int64(masked), nil //nolint:gosec // 已通过掩码确保范围安全。
		}

		slog.Warn("Sonyflake generator failed, retrying...", "retry", i+1, "error", err)
		time.Sleep(time.Millisecond * 10)
	}

	return 0, ErrGeneratorFailed
}

// DefaultGenerator 默认全局 ID 生成器.
type DefaultGenerator struct {
	mu sync.Mutex
	ts int64
	id int64
}

// Generate 生成简单的递增 ID（线程安全）.
func (g *DefaultGenerator) Generate() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixNano()
	if now <= g.ts {
		g.id++
	} else {
		g.ts = now
		g.id = 0
	}

	return now + g.id
}

var defaultGenerator = &DefaultGenerator{}

// NextID 全局快捷生成入口.
func GenID() uint64 {
	generatedID := defaultGenerator.Generate()
	if generatedID < 0 {
		return uint64(-generatedID)
	}
	return uint64(generatedID)
}

// GenOrderNo 生成订单号，格式为 "O" + 唯一ID.
func GenOrderNo() string {
	return fmt.Sprintf("O%d", GenID())
}
