// Package idgen 提供了分布式唯一 ID 生成器的实现.
// 支持 Snowflake 和 Sonyflake 两种算法，可通过配置选择.
package idgen

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/snowflake"
	"github.com/sony/sonyflake"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/utils"
)

var (
	// ErrUnsupportedType 不支持的 ID 生成器类型.
	ErrUnsupportedType = errors.New("unsupported id generator type")
	// ErrParseTime 解析时间失败.
	ErrParseTime = errors.New("failed to parse start time")
	// ErrCreateNode 创建 Snowflake 节点失败.
	ErrCreateNode = errors.New("failed to create snowflake node")
	// ErrCreateSonyflake 创建 Sonyflake 实例失败.
	ErrCreateSonyflake = errors.New("failed to create sonyflake instance")
	// ErrInvalidMachineID 错误的机器 ID.
	ErrInvalidMachineID = errors.New("machine_id must be between 0 and 65535")
)

const (
	msPerSecond = 1000000
	maxRetries  = 3
)

// Generator 定义 ID 生成器接口.
type Generator interface {
	Generate() int64
}

// SnowflakeGenerator 使用雪花算法实现 Generator.
// 特点：每毫秒可生成 4096 个 ID，支持 1024 台机器，可用约 69 年.
type SnowflakeGenerator struct { // 雪花算法生成器，已对齐。
	node *snowflake.Node
}

// NewSnowflakeGenerator 创建一个新的 SnowflakeGenerator.
func NewSnowflakeGenerator(cfg config.SnowflakeConfig) (*SnowflakeGenerator, error) {
	if cfg.StartTime != "" {
		st, err := time.Parse("2006-01-02", cfg.StartTime)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrParseTime, err)
		}
		snowflake.Epoch = st.UnixNano() / msPerSecond
	}

	node, err := snowflake.NewNode(cfg.MachineID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateNode, err)
	}

	slog.Info("snowflake generator initialized", "machine_id", cfg.MachineID, "epoch", snowflake.Epoch)

	return &SnowflakeGenerator{
		node: node,
	}, nil
}

// Generate 生成一个新的 ID.
func (g *SnowflakeGenerator) Generate() int64 {
	return g.node.Generate().Int64()
}

// SonyflakeGenerator 使用 Sonyflake 算法实现 Generator.
// 特点：每 10 毫秒可生成 256 个 ID，支持 65536 台机器，可用约 174 年.
type SonyflakeGenerator struct { // Sonyflake 生成器，已对齐。
	sf *sonyflake.Sonyflake
}

// NewSonyflakeGenerator 创建一个新的 SonyflakeGenerator.
func NewSonyflakeGenerator(cfg config.SnowflakeConfig) (*SonyflakeGenerator, error) {
	var startTime time.Time
	if cfg.StartTime != "" {
		st, err := time.Parse("2006-01-02", cfg.StartTime)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrParseTime, err)
		}
		startTime = st
	} else {
		startTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	if cfg.MachineID < 0 || cfg.MachineID > 65535 {
		return nil, ErrInvalidMachineID
	}

	settings := sonyflake.Settings{
		StartTime: startTime,
		MachineID: func() (uint16, error) {
			// G115 fix: Explicitly mask to correct size before casting
			mid := cfg.MachineID & 0xFFFF
			// G115 fix
			return utils.Int64ToUint16(mid), nil
		},
	}

	sonyFlake, err := sonyflake.New(settings)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateSonyflake, err)
	}

	slog.Info("sonyflake generator initialized", "machine_id", cfg.MachineID, "start_time", startTime)

	return &SonyflakeGenerator{
		sf: sonyFlake,
	}, nil
}

// Generate 生成一个新的 ID.
func (g *SonyflakeGenerator) Generate() int64 {
	for i := range maxRetries {
		id, err := g.sf.NextID()
		if err == nil {
			// G115 fix: Mask before cast
			maskedID := id & 0x7FFFFFFFFFFFFFFF
			// G115 fix
			return utils.Uint64ToInt64(maskedID)
		}

		slog.Warn("Sonyflake generator failed, retrying...", "retry", i+1, "error", err)
		time.Sleep(10 * time.Millisecond)
	}

	slog.Error("Sonyflake generator failed after multiple retries")

	return 0
}

// NewGenerator 根据配置创建对应类型的 ID 生成器.
func NewGenerator(cfg config.SnowflakeConfig) (Generator, error) {
	switch cfg.Type {
	case "sonyflake":
		return NewSonyflakeGenerator(cfg)
	case "snowflake", "":
		return NewSnowflakeGenerator(cfg)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedType, cfg.Type)
	}
}

// 全局默认生成器.
var (
	defaultGenerator Generator
	once             sync.Once
)

// Init 初始化全局默认生成器.
func Init(cfg config.SnowflakeConfig) error {
	var err error
	once.Do(func() {
		defaultGenerator, err = NewGenerator(cfg)
	})

	return err
}

// Default 返回全局默认生成器实例.
func Default() Generator {
	if defaultGenerator == nil {
		if err := Init(config.SnowflakeConfig{MachineID: 1}); err != nil {
			slog.Error("failed to initialize default id generator", "error", err)
		}
	}

	return defaultGenerator
}

// GenID 使用默认生成器生成全局唯一的 uint64 ID.
func GenID() uint64 {
	if defaultGenerator == nil {
		if err := Init(config.SnowflakeConfig{MachineID: 1}); err != nil {
			panic(fmt.Errorf("failed to auto-initialize default id generator: %w", err))
		}
	}

	generatedID := defaultGenerator.Generate()
	// G115 fix: Safe cast
	return utils.Int64ToUint64(generatedID & 0x7FFFFFFFFFFFFFFF)
}

// GenOrderNo 生成订单号，格式为 "O" + 唯一ID.
func GenOrderNo() string {
	return "O" + strconv.FormatUint(GenID(), 10)
}

// GenPaymentNo 生成支付单号.
func GenPaymentNo() string {
	return "P" + strconv.FormatUint(GenID(), 10)
}

// GenRefundNo 生成退款单号.
func GenRefundNo() string {
	return "R" + strconv.FormatUint(GenID(), 10)
}

// GenSPUNo 生成 SPU 编号.
func GenSPUNo() string {
	return "SPU" + strconv.FormatUint(GenID(), 10)
}

// GenSKUNo 生成 SKU 编号.
func GenSKUNo() string {
	return "SKU" + strconv.FormatUint(GenID(), 10)
}

// GenCouponCode 生成优惠券码.
func GenCouponCode() string {
	return "C" + strconv.FormatUint(GenID(), 10)
}
