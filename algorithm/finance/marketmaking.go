package finance

import (
	"math"

	"github.com/shopspring/decimal"
)

// AvellanedaStoikovModel 市场做市模型核心逻辑
type AvellanedaStoikovModel struct {
	Gamma float64 // 风险厌恶
	Sigma float64 // 波动率
	Kappa float64 // 订单流密度
	T     float64 // 终端时间
}

type ASQuote struct {
	ReservationPrice decimal.Decimal
	Spread           decimal.Decimal
	Bid              decimal.Decimal
	Ask              decimal.Decimal
}

func (m *AvellanedaStoikovModel) Calculate(mid, q, timeRemaining float64) ASQuote {
	resPrice := mid - q*m.Gamma*math.Pow(m.Sigma, 2)*timeRemaining

	spread := m.Gamma*math.Pow(m.Sigma, 2)*timeRemaining + (2.0/m.Gamma)*math.Log(1.0+(m.Gamma/m.Kappa))

	resDec := decimal.NewFromFloat(resPrice)
	spreadDec := decimal.NewFromFloat(spread)
	halfSpread := spreadDec.Div(decimal.NewFromFloat(2))

	return ASQuote{
		ReservationPrice: resDec,
		Spread:           spreadDec,
		Bid:              resDec.Sub(halfSpread),
		Ask:              resDec.Add(halfSpread),
	}
}
