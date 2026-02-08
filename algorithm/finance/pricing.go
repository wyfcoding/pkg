// Package finance - 期权定价算法（Black-Scholes 模型）。
package finance

import (
	"math"

	"github.com/shopspring/decimal"
	"github.com/wyfcoding/pkg/algorithm/types"
	"github.com/wyfcoding/pkg/xerrors"
)

// BlackScholesCalculator Black-Scholes 期权定价计算器。
type BlackScholesCalculator struct{}

// NewBlackScholesCalculator 创建 Black-Scholes 计算器。
func NewBlackScholesCalculator() *BlackScholesCalculator {
	return &BlackScholesCalculator{}
}

// CalculateCallPrice 计算看涨期权价格。
func (bsc *BlackScholesCalculator) CalculateCallPrice(spot, strike, expiry, rate, vol, div decimal.Decimal) (decimal.Decimal, error) {
	if spot.LessThanOrEqual(decimal.Zero) || strike.LessThanOrEqual(decimal.Zero) || expiry.LessThanOrEqual(decimal.Zero) || vol.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, xerrors.ErrInvalidInput
	}
	sFloat := spot.InexactFloat64()
	kFloat := strike.InexactFloat64()
	tFloat := expiry.InexactFloat64()
	rFloat := rate.InexactFloat64()
	sigmaFloat := vol.InexactFloat64()
	qFloat := div.InexactFloat64()
	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	d2 := d1 - sigmaFloat*math.Sqrt(tFloat)
	callPrice := sFloat*math.Exp(-qFloat*tFloat)*normCDF(d1) - kFloat*math.Exp(-rFloat*tFloat)*normCDF(d2)
	return decimal.NewFromFloat(callPrice), nil
}

// CalculatePutPrice 计算看跌期权价格。
func (bsc *BlackScholesCalculator) CalculatePutPrice(spot, strike, expiry, rate, vol, div decimal.Decimal) (decimal.Decimal, error) {
	if spot.LessThanOrEqual(decimal.Zero) || strike.LessThanOrEqual(decimal.Zero) || expiry.LessThanOrEqual(decimal.Zero) || vol.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, xerrors.ErrInvalidInput
	}
	sFloat := spot.InexactFloat64()
	kFloat := strike.InexactFloat64()
	tFloat := expiry.InexactFloat64()
	rFloat := rate.InexactFloat64()
	sigmaFloat := vol.InexactFloat64()
	qFloat := div.InexactFloat64()
	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	d2 := d1 - sigmaFloat*math.Sqrt(tFloat)
	putPrice := kFloat*math.Exp(-rFloat*tFloat)*normCDF(-d2) - sFloat*math.Exp(-qFloat*tFloat)*normCDF(-d1)
	return decimal.NewFromFloat(putPrice), nil
}

// CalculateDelta 计算 Delta。
func (bsc *BlackScholesCalculator) CalculateDelta(optionType string, spot, strike, expiry, rate, vol, div decimal.Decimal) (decimal.Decimal, error) {
	sFloat := spot.InexactFloat64()
	kFloat := strike.InexactFloat64()
	tFloat := expiry.InexactFloat64()
	rFloat := rate.InexactFloat64()
	sigmaFloat := vol.InexactFloat64()
	qFloat := div.InexactFloat64()
	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	var delta float64
	switch types.OptionType(optionType) {
	case types.OptionTypeCall:
		delta = math.Exp(-qFloat*tFloat) * normCDF(d1)
	case types.OptionTypePut:
		delta = math.Exp(-qFloat*tFloat) * (normCDF(d1) - 1)
	default:
		return decimal.Zero, xerrors.ErrInvalidOptionType
	}
	return decimal.NewFromFloat(delta), nil
}

// CalculateGamma 计算 Gamma。
func (bsc *BlackScholesCalculator) CalculateGamma(spot, strike, expiry, rate, vol, div decimal.Decimal) (decimal.Decimal, error) {
	sFloat := spot.InexactFloat64()
	kFloat := strike.InexactFloat64()
	tFloat := expiry.InexactFloat64()
	rFloat := rate.InexactFloat64()
	sigmaFloat := vol.InexactFloat64()
	qFloat := div.InexactFloat64()
	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	gamma := math.Exp(-qFloat*tFloat) * normPDF(d1) / (sFloat * sigmaFloat * math.Sqrt(tFloat))
	return decimal.NewFromFloat(gamma), nil
}

// CalculateVega 计算 Vega。
func (bsc *BlackScholesCalculator) CalculateVega(spot, strike, expiry, rate, vol, div decimal.Decimal) (decimal.Decimal, error) {
	sFloat := spot.InexactFloat64()
	kFloat := strike.InexactFloat64()
	tFloat := expiry.InexactFloat64()
	rFloat := rate.InexactFloat64()
	sigmaFloat := vol.InexactFloat64()
	qFloat := div.InexactFloat64()
	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	vega := sFloat * math.Exp(-qFloat*tFloat) * normPDF(d1) * math.Sqrt(tFloat) / 100
	return decimal.NewFromFloat(vega), nil
}

// CalculateTheta 计算 Theta。
func (bsc *BlackScholesCalculator) CalculateTheta(optionType string, spot, strike, expiry, rate, vol, div decimal.Decimal) (decimal.Decimal, error) {
	sFloat := spot.InexactFloat64()
	kFloat := strike.InexactFloat64()
	tFloat := expiry.InexactFloat64()
	rFloat := rate.InexactFloat64()
	sigmaFloat := vol.InexactFloat64()
	qFloat := div.InexactFloat64()
	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	d2 := d1 - sigmaFloat*math.Sqrt(tFloat)
	var theta float64
	switch types.OptionType(optionType) {
	case types.OptionTypeCall:
		theta = -sFloat*math.Exp(-qFloat*tFloat)*normPDF(d1)*sigmaFloat/(2*math.Sqrt(tFloat)) - rFloat*kFloat*math.Exp(-rFloat*tFloat)*normCDF(d2) + qFloat*sFloat*math.Exp(-qFloat*tFloat)*normCDF(d1)
	case types.OptionTypePut:
		theta = -sFloat*math.Exp(-qFloat*tFloat)*normPDF(d1)*sigmaFloat/(2*math.Sqrt(tFloat)) + rFloat*kFloat*math.Exp(-rFloat*tFloat)*normCDF(-d2) - qFloat*sFloat*math.Exp(-qFloat*tFloat)*normCDF(-d1)
	default:
		return decimal.Zero, xerrors.ErrInvalidOptionType
	}
	return decimal.NewFromFloat(theta / 365), nil // 每日 theta。
}

// BlackScholesResult 包含计算出的期权价格及其希腊字母。
type BlackScholesResult struct {
	Price decimal.Decimal
	Delta decimal.Decimal
	Gamma decimal.Decimal
	Vega  decimal.Decimal
	Theta decimal.Decimal
	Rho   decimal.Decimal
}

// Calculate 一次性计算期权价格及所有希腊字母。
func (bsc *BlackScholesCalculator) Calculate(optionType string, spot, strike, expiry, rate, vol, div decimal.Decimal) (*BlackScholesResult, error) {
	if spot.LessThanOrEqual(decimal.Zero) || strike.LessThanOrEqual(decimal.Zero) || expiry.LessThanOrEqual(decimal.Zero) || vol.LessThanOrEqual(decimal.Zero) {
		return nil, xerrors.ErrInvalidInput
	}

	s := spot.InexactFloat64()
	k := strike.InexactFloat64()
	t := expiry.InexactFloat64()
	r := rate.InexactFloat64()
	sigma := vol.InexactFloat64()
	q := div.InexactFloat64()

	d1 := (math.Log(s/k) + (r-q+0.5*sigma*sigma)*t) / (sigma * math.Sqrt(t))
	d2 := d1 - sigma*math.Sqrt(t)

	n_d1 := normCDF(d1)
	n_d2 := normCDF(d2)
	exp_rt := math.Exp(-r * t)
	exp_qt := math.Exp(-q * t)
	phi_d1 := normPDF(d1)

	res := &BlackScholesResult{}

	isCall := types.OptionType(optionType) == types.OptionTypeCall
	if isCall {
		res.Price = decimal.NewFromFloat(s*exp_qt*n_d1 - k*exp_rt*n_d2)
		res.Delta = decimal.NewFromFloat(exp_qt * n_d1)
		res.Theta = decimal.NewFromFloat((-s*exp_qt*phi_d1*sigma/(2*math.Sqrt(t)) - r*k*exp_rt*n_d2 + q*s*exp_qt*n_d1) / 365)
		res.Rho = decimal.NewFromFloat(k * t * exp_rt * n_d2 / 100)
	} else {
		res.Price = decimal.NewFromFloat(k*exp_rt*normCDF(-d2) - s*exp_qt*normCDF(-d1))
		res.Delta = decimal.NewFromFloat(exp_qt * (n_d1 - 1))
		res.Theta = decimal.NewFromFloat((-s*exp_qt*phi_d1*sigma/(2*math.Sqrt(t)) + r*k*exp_rt*normCDF(-d2) - q*s*exp_qt*normCDF(-d1)) / 365)
		res.Rho = decimal.NewFromFloat(-k * t * exp_rt * normCDF(-d2) / 100)
	}

	res.Gamma = decimal.NewFromFloat(exp_qt * phi_d1 / (s * sigma * math.Sqrt(t)))
	res.Vega = decimal.NewFromFloat(s * exp_qt * phi_d1 * math.Sqrt(t) / 100)

	return res, nil
}

// CalculateRho 计算 Rho。
func (bsc *BlackScholesCalculator) CalculateRho(optionType string, spot, strike, expiry, rate, vol, div decimal.Decimal) (decimal.Decimal, error) {
	sFloat := spot.InexactFloat64()
	kFloat := strike.InexactFloat64()
	tFloat := expiry.InexactFloat64()
	rFloat := rate.InexactFloat64()
	sigmaFloat := vol.InexactFloat64()
	d1 := (math.Log(sFloat/kFloat) + (rFloat-div.InexactFloat64()+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	d2 := d1 - sigmaFloat*math.Sqrt(tFloat)
	var rho float64
	switch types.OptionType(optionType) {
	case types.OptionTypeCall:
		rho = kFloat * tFloat * math.Exp(-rFloat*tFloat) * normCDF(d2) / 100
	case types.OptionTypePut:
		rho = -kFloat * tFloat * math.Exp(-rFloat*tFloat) * normCDF(-d2) / 100
	default:
		return decimal.Zero, xerrors.ErrInvalidOptionType
	}
	return decimal.NewFromFloat(rho), nil
}

func normCDF(x float64) float64 {
	return (1.0 + math.Erf(x/math.Sqrt2)) / 2.0
}

func normPDF(x float64) float64 {
	return math.Exp(-x*x/2) / math.Sqrt(2*math.Pi)
}

// CalculateImpliedVolatility 计算隐含波动率。
func (bsc *BlackScholesCalculator) CalculateImpliedVolatility(optionType string, spot, strike, expiry, rate, div, marketPrice decimal.Decimal) (decimal.Decimal, error) {
	sFloat := spot.InexactFloat64()
	kFloat := strike.InexactFloat64()
	tFloat := expiry.InexactFloat64()
	rFloat := rate.InexactFloat64()
	qFloat := div.InexactFloat64()
	marketPriceFloat := marketPrice.InexactFloat64()
	sigma := 0.3
	tolerance := 0.0001
	maxIterations := 100
	for range maxIterations {
		d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigma*sigma)*tFloat) / (sigma * math.Sqrt(tFloat))
		d2 := d1 - sigma*math.Sqrt(tFloat)
		var price, vega float64
		if types.OptionType(optionType) == types.OptionTypeCall {
			price = sFloat*math.Exp(-qFloat*tFloat)*normCDF(d1) - kFloat*math.Exp(-rFloat*tFloat)*normCDF(d2)
		} else {
			price = kFloat*math.Exp(-rFloat*tFloat)*normCDF(-d2) - sFloat*math.Exp(-qFloat*tFloat)*normCDF(-d1)
		}
		vega = sFloat * math.Exp(-qFloat*tFloat) * normPDF(d1) * math.Sqrt(tFloat)
		diff := price - marketPriceFloat
		if math.Abs(diff) < tolerance {
			return decimal.NewFromFloat(sigma), nil
		}
		if vega == 0 {
			break
		}
		sigma -= diff / vega
		if sigma < 0 {
			sigma = 0.001
		}
	}
	return decimal.NewFromFloat(sigma), nil
}
