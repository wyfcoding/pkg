// Package idgen 提供了分布式唯一 ID 生成器的实现.
// 支持 Snowflake 和 Sonyflake 两种算法，可通过配置选择.
package idgen

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/snowflake"
	"github.com/sony/sonyflake"
	"github.com/wyfcoding/pkg/cast"
	"github.com/wyfcoding/pkg/config"
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
	// ErrInvalidSnowflakeMachineID Snowflake 机器 ID 错误.
	ErrInvalidSnowflakeMachineID = errors.New("snowflake machine_id must be between 0 and 1023")
)

const (
	msPerSecond           = 1000000
	maxRetries            = 3
	sonyflakeDefaultEpoch = "2020-01-01"
)

// Generator 定义 ID 生成器接口.
type Generator interface {
	Generate() int64
}

// SnowflakeGenerator 使用雪花算法实现 Generator.
type SnowflakeGenerator struct {
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

	if cfg.MachineID < 0 || cfg.MachineID > 1023 {
		return nil, ErrInvalidSnowflakeMachineID
	}

	node, err := snowflake.NewNode(cfg.MachineID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateNode, err)
	}

	slog.Info("snowflake generator initialized", "machine_id", cfg.MachineID, "epoch", snowflake.Epoch)

	return &SnowflakeGenerator{node: node}, nil
}

func (g *SnowflakeGenerator) Generate() int64 {
	return g.node.Generate().Int64()
}

// SonyflakeGenerator 使用 Sonyflake 算法实现 Generator.
type SonyflakeGenerator struct {
	sf *sonyflake.Sonyflake
}

func NewSonyflakeGenerator(cfg config.SnowflakeConfig) (*SonyflakeGenerator, error) {
	var startTime time.Time
	startTimeStr := cfg.StartTime
	if startTimeStr == "" {
		startTimeStr = sonyflakeDefaultEpoch
	}

	st, err := time.Parse("2006-01-02", startTimeStr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseTime, err)
	}
	startTime = st

	if cfg.MachineID < 0 || cfg.MachineID > 65535 {
		return nil, ErrInvalidMachineID
	}

	settings := sonyflake.Settings{
		StartTime: startTime,
		MachineID: func() (uint16, error) {
			return cast.Int64ToUint16(cfg.MachineID & 0xFFFF), nil
		},
	}

	sf, err := sonyflake.New(settings)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateSonyflake, err)
	}

	slog.Info("sonyflake generator initialized", "machine_id", cfg.MachineID, "start_time", startTime)

	return &SonyflakeGenerator{sf: sf}, nil
}

func (g *SonyflakeGenerator) Generate() int64 {
	for range maxRetries {
		id, err := g.sf.NextID()
		if err == nil {
			return cast.Uint64ToInt64(id & 0x7FFFFFFFFFFFFFFF)
		}
		time.Sleep(10 * time.Millisecond)
	}
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

var (
	defaultGenerator Generator
	once             sync.Once
)

// Init 初始化全局默认生成器.
func Init(cfg config.SnowflakeConfig) error {
	var err error
	once.Do(func() {
		if cfg.MachineID <= 0 {
			if envMid := os.Getenv("MACHINE_ID"); envMid != "" {
				if id, parseErr := strconv.ParseInt(envMid, 10, 64); parseErr == nil {
					cfg.MachineID = id
				}
			}
		}
		if cfg.MachineID <= 0 {
			cfg.MachineID = 1 // 兜底
		}
		defaultGenerator, err = NewGenerator(cfg)
	})
	return err
}

func GenID() uint64 {
	if defaultGenerator == nil {
		_ = Init(config.SnowflakeConfig{MachineID: 1})
	}
	return cast.Int64ToUint64(defaultGenerator.Generate() & 0x7FFFFFFFFFFFFFFF)
}

func GenIDString() string {
	return strconv.FormatUint(GenID(), 10)
}

func GenOrderNo() string {
	id := GenID()
	return "O" + strconv.FormatUint(id, 10)
}

func GenPaymentNo() string {
	id := GenID()
	return "P" + strconv.FormatUint(id, 10)
}

func GenRefundNo() string {
	id := GenID()
	return "R" + strconv.FormatUint(id, 10)
}

func GenSPUNo() string {
	id := GenID()
	return "SPU" + strconv.FormatUint(id, 10)
}

func GenSKUNo() string {
	id := GenID()
	return "SKU" + strconv.FormatUint(id, 10)
}

func GenCouponCode() string {
	id := GenID()
	return "C" + strconv.FormatUint(id, 10)
}
