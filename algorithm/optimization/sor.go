package optimization

import (
	"sort"

	"github.com/shopspring/decimal"
)

type RouteInput struct {
	VenueID   string
	Price     decimal.Decimal
	Quantity  decimal.Decimal
	FeeRate   decimal.Decimal
	LatencyMs float64
}

type RouteOutput struct {
	VenueID  string
	Price    decimal.Decimal
	Quantity decimal.Decimal
}

type SOROptimizer struct {
	LatencyFactor float64
}

func (o *SOROptimizer) Optimize(totalQty decimal.Decimal, inputs []RouteInput, isBuy bool) []RouteOutput {
	type effectiveNode struct {
		effPrice decimal.Decimal
		input    RouteInput
	}

	nodes := make([]effectiveNode, len(inputs))
	for i, in := range inputs {
		// effPrice = Price * (1 ± Fee) + LatencyCost
		feeFactor := decimal.NewFromFloat(1.0)
		if isBuy {
			feeFactor = feeFactor.Add(in.FeeRate)
		} else {
			feeFactor = feeFactor.Sub(in.FeeRate)
		}

		eff := in.Price.Mul(feeFactor)
		latencyCost := decimal.NewFromFloat(in.LatencyMs * o.LatencyFactor)
		if isBuy {
			eff = eff.Add(latencyCost)
		} else {
			eff = eff.Sub(latencyCost)
		}

		nodes[i] = effectiveNode{input: in, effPrice: eff}
	}

	sort.Slice(nodes, func(i, j int) bool {
		if isBuy {
			return nodes[i].effPrice.LessThan(nodes[j].effPrice)
		}
		return nodes[i].effPrice.GreaterThan(nodes[j].effPrice)
	})

	remaining := totalQty
	venueGroups := make(map[string][]RouteOutput)

	for _, n := range nodes {
		if remaining.IsZero() {
			break
		}
		fill := decimal.Min(remaining, n.input.Quantity)

		venueGroups[n.input.VenueID] = append(venueGroups[n.input.VenueID], RouteOutput{
			VenueID:  n.input.VenueID,
			Price:    n.input.Price,
			Quantity: fill,
		})
		remaining = remaining.Sub(fill)
	}

	// VWAP 合并各 Venue 的结果
	final := make([]RouteOutput, 0, len(venueGroups))
	for venueID, group := range venueGroups {
		var totalV, totalQ decimal.Decimal
		for _, r := range group {
			totalV = totalV.Add(r.Price.Mul(r.Quantity))
			totalQ = totalQ.Add(r.Quantity)
		}
		final = append(final, RouteOutput{
			VenueID:  venueID,
			Price:    totalV.Div(totalQ),
			Quantity: totalQ,
		})
	}

	return final
}
