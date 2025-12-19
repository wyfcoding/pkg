package idgen

import (
	"fmt"
	"sync"
	"time"

	"github.com/wyfcoding/pkg/config"

	"github.com/bwmarrin/snowflake"
)

// Generator 定义 ID 生成器接口。
type Generator interface {
	Generate() int64
}

// SnowflakeGenerator 使用雪花算法实现 Generator。
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

	return &SnowflakeGenerator{
		node: node,
	}, nil
}

// Generate 生成一个新的 ID。
func (g *SnowflakeGenerator) Generate() int64 {
	return g.node.Generate().Int64()
}

// 全局默认生成器
var (
	defaultGenerator *SnowflakeGenerator
	once             sync.Once
)

// Init 初始化全局默认生成器。
// 此函数应在应用程序启动时调用，以配置全局唯一的ID生成规则。
func Init(cfg config.SnowflakeConfig) error {
	var err error
	// 使用sync.Once确保初始化操作只执行一次。
	once.Do(func() {
		defaultGenerator, err = NewSnowflakeGenerator(cfg)
	})
	return err
}

// GenID 使用默认生成器生成唯一 ID。
// 如果默认生成器尚未初始化，它会使用默认配置（MachineID为1）进行一次回退初始化。
func GenID() uint64 {
	if defaultGenerator == nil {
		// 如果未初始化，则使用默认值进行回退初始化。
		_ = Init(config.SnowflakeConfig{MachineID: 1})
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
