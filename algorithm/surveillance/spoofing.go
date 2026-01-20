package surveillance

import (
	"time"

	"github.com/shopspring/decimal"
)

type MarketEvent struct {
	UserID    string
	Type      string // PLACE, CANCEL, FILL
	Price     decimal.Decimal
	Quantity  decimal.Decimal
	Timestamp time.Time
}

type SurveillanceEngine struct {
	Threshold decimal.Decimal
	Window    time.Duration
}

func (e *SurveillanceEngine) Analyze(events []MarketEvent) (score float64, reason string) {
	if len(events) == 0 {
		return 0, "No events"
	}

	var cancelCount, fillCount int
	var largeCancels int
	levels := make(map[string]int)

	for _, ev := range events {
		switch ev.Type {
		case "CANCEL":
			cancelCount++
			if ev.Quantity.GreaterThanOrEqual(e.Threshold) {
				largeCancels++
			}
		case "FILL":
			fillCount++
		case "PLACE":
			if ev.Quantity.GreaterThanOrEqual(e.Threshold) {
				levels[ev.Price.String()]++
			}
		}
	}

	// 维度 1: Spoofing (频繁大额撤单，且无成交)
	spoofScore := 0.0
	if largeCancels > 0 {
		ltr := float64(fillCount) / float64(largeCancels+fillCount)
		if ltr < 0.1 { // 成交比低于 10%
			spoofScore = 0.8
		}
	}

	// 维度 2: Layering (多价格档位同时挂大单)
	layerScore := 0.0
	if len(levels) >= 3 {
		layerScore = 0.6
	}

	score = mathMax(spoofScore, layerScore)
	if score > 0.7 {
		reason = "Aggressive market manipulation pattern"
	} else {
		reason = "Normal market participation"
	}

	return score, reason
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
