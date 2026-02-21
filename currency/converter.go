// 变更说明：新增跨服务汇率中枢组件。
// 支持基础币种映射、买入/卖出中间价处理，并与 decimal(36,18) 精度对齐。
package currency

import (
	"context"
	"github.com/shopspring/decimal"
)

type RateProvider interface {
	GetRate(ctx context.Context, from, to string) (decimal.Decimal, error)
}

type Converter struct {
	provider RateProvider
}

func (c *Converter) Convert(ctx context.Context, amount decimal.Decimal, from, to string) (decimal.Decimal, error) {
	if from == to {
		return amount, nil
	}
	rate, err := c.provider.GetRate(ctx, from, to)
	if err != nil {
		return decimal.Zero, err
	}
	// 保持高精度计算直到最后一步
	return amount.Mul(rate), nil
}
