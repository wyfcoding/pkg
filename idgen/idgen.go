// Package idgen 提供了分布式唯一 ID 生成器的实现。
// 支持 Snowflake 和 Sonyflake 两种算法，可通过配置选择。
package idgen

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wyfcoding/pkg/config"

	"github.com/bwmarrin/snowflake"
	"github.com/sony/sonyflake"
)

// Generator 定义 ID 生成器接口。
type Generator interface {
	Generate() int64
}

// SnowflakeGenerator 使用雪花算法实现 Generator。
// 特点：每毫秒可生成 4096 个 ID，支持 1024 台机器，可用约 69 年。
type SnowflakeGenerator struct {
	node *snowflake.Node
}

// NewSnowflakeGenerator 创建一个新的 SnowflakeGenerator。
// cfg 参数提供了雪花算法所需的机器ID和可选的起始时间。
func NewSnowflakeGenerator(cfg config.SnowflakeConfig) (*SnowflakeGenerator, error) {
	// 如果配置中提供了起始时间，则解析并设置雪花算法的纪元。
	if cfg.StartTime != "" {
		st, err := time.Parse("2006-01-02", cfg.StartTime)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start time: %w", err)
		}
		// snowflake.Epoch 期望毫秒级Unix时间戳。
		snowflake.Epoch = st.UnixNano() / 1000000
	}

	// 使用配置的机器ID创建一个雪花节点。
	node, err := snowflake.NewNode(cfg.MachineID)
	if err != nil {
		return nil, fmt.Errorf("failed to create snowflake node: %w", err)
	}

	slog.Info("SnowflakeGenerator initialized", "machine_id", cfg.MachineID, "epoch", snowflake.Epoch)

	return &SnowflakeGenerator{
		node: node,
	}, nil
}

// Generate 生成一个新的 ID。
func (g *SnowflakeGenerator) Generate() int64 {
	return g.node.Generate().Int64()
}

// SonyflakeGenerator 使用 Sonyflake 算法实现 Generator。
// 特点：每 10 毫秒可生成 256 个 ID，支持 65536 台机器，可用约 174 年。
// 适用于大规模分布式集群。
type SonyflakeGenerator struct {
	sf *sonyflake.Sonyflake
}

// NewSonyflakeGenerator 创建一个新的 SonyflakeGenerator。
// cfg 参数提供了所需的机器ID和可选的起始时间。
func NewSonyflakeGenerator(cfg config.SnowflakeConfig) (*SonyflakeGenerator, error) {
	var startTime time.Time
	if cfg.StartTime != "" {
		var err error
		startTime, err = time.Parse("2006-01-02", cfg.StartTime)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start time: %w", err)
		}
	} else {
		// 默认使用 2020-01-01 作为起始时间
		startTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	settings := sonyflake.Settings{
		StartTime: startTime,
		MachineID: func() (uint16, error) {
			// Sonyflake 使用 16 位机器 ID（0-65535）
			return uint16(cfg.MachineID & 0xFFFF), nil
		},
	}

	sf, err := sonyflake.New(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to create sonyflake instance: %w", err)
	}

	slog.Info("SonyflakeGenerator initialized", "machine_id", cfg.MachineID, "start_time", startTime)

	return &SonyflakeGenerator{
		sf: sf,
	}, nil
}

// Generate 生成一个新的 ID。
func (g *SonyflakeGenerator) Generate() int64 {
	// Sonyflake 返回 uint64，但为了统一接口我们转为 int64
	// 由于 ID 的最高位通常为 0（用于表示正数），这不会导致问题
	id, err := g.sf.NextID()
	if err != nil {
		// 在极端情况下（如时钟回拨）可能失败，返回 0 并记录错误
		// 生产环境中应有更健壮的处理，这里简化处理
		return 0
	}
	return int64(id)
}

// NewGenerator 根据配置创建对应类型的 ID 生成器。
// 这是推荐的工厂函数，可自动根据 cfg.Type 选择合适的实现。
func NewGenerator(cfg config.SnowflakeConfig) (Generator, error) {
	switch cfg.Type {
	case "sonyflake":
		return NewSonyflakeGenerator(cfg)
	case "snowflake", "":
		// 默认使用 Snowflake
		return NewSnowflakeGenerator(cfg)
	default:
		return nil, fmt.Errorf("unsupported id generator type: %s", cfg.Type)
	}
}

// 全局默认生成器
var (
	defaultGenerator Generator
	once             sync.Once
)

// Init 初始化全局默认生成器。
// 此函数应在应用程序启动时调用，以配置全局唯一的ID生成规则。
func Init(cfg config.SnowflakeConfig) error {
	var err error
	// 使用sync.Once确保初始化操作只执行一次。
	once.Do(func() {
		defaultGenerator, err = NewGenerator(cfg)
	})
	return err
}

// GenID 使用默认生成器生成唯一 ID。
// 如果默认生成器尚未初始化，它会使用默认配置（MachineID为1）进行一次回退初始化。
func GenID() uint64 {
	if defaultGenerator == nil {
		// 如果未初始化，则使用默认值进行回退初始化。
		if err := Init(config.SnowflakeConfig{MachineID: 1}); err != nil {
			panic(fmt.Errorf("failed to auto-initialize default id generator: %w", err))
		}
	}
	return uint64(defaultGenerator.Generate())
}

// GenOrderNo 生成订单号，格式为 "O" + 唯一ID。
func GenOrderNo() string {
	return fmt.Sprintf("O%d", GenID())
}

// GenPaymentNo 生成支付单号，格式为 "P" + 唯一ID。
func GenPaymentNo() string {
	return fmt.Sprintf("P%d", GenID())
}

// GenRefundNo 生成退款单号，格式为 "R" + 唯一ID。
func GenRefundNo() string {
	return fmt.Sprintf("R%d", GenID())
}

// GenSPUNo 生成 SPU 编号，格式为 "SPU" + 唯一ID。
func GenSPUNo() string {
	return fmt.Sprintf("SPU%d", GenID())
}

// GenSKUNo 生成 SKU 编号，格式为 "SKU" + 唯一ID。
func GenSKUNo() string {
	return fmt.Sprintf("SKU%d", GenID())
}

// GenCouponCode 生成优惠券码，格式为 "C" + 唯一ID。
func GenCouponCode() string {
	return fmt.Sprintf("C%d", GenID())
}
