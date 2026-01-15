// Package system 提供了系统监控、日志与基础元数据模型.
package system

import (
	"github.com/shopspring/decimal"
)

// MetricsData 基础资源监控指标数据.
type MetricsData struct {
	CPUUsage          decimal.Decimal `json:"cpu_usage"`
	MemoryUsage       decimal.Decimal `json:"memory_usage"`
	DiskUsage         decimal.Decimal `json:"disk_usage"`
	NetworkThroughput decimal.Decimal `json:"network_throughput"`
	AverageLatency    decimal.Decimal `json:"average_latency"`
	ErrorRate         decimal.Decimal `json:"error_rate"`
	Timestamp         int64           `json:"timestamp"`
	ActiveConnections int64           `json:"active_connections"`
	RequestsPerSecond int64           `json:"requests_per_second"`
}

// TradeStatisticsData 交易统计汇总数据.
type TradeStatisticsData struct {
	TotalVolume  decimal.Decimal `json:"total_volume"`
	TotalValue   decimal.Decimal `json:"total_value"`
	AveragePrice decimal.Decimal `json:"average_price"`
	HighPrice    decimal.Decimal `json:"high_price"`
	LowPrice     decimal.Decimal `json:"low_price"`
	OpenPrice    decimal.Decimal `json:"open_price"`
	ClosePrice   decimal.Decimal `json:"close_price"`
	Symbol       string          `json:"symbol"`
	TotalTrades  int64           `json:"total_trades"`
	Timestamp    int64           `json:"timestamp"`
}
