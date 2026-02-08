package grpcclient

import (
	"strconv"
	"sync/atomic"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
)

const weightedRRName = "weighted_rr"

func init() {
	balancer.Register(base.NewBalancerBuilder(weightedRRName, &weightedRRPickerBuilder{}, base.Config{HealthCheck: true}))
}

type weightedRRPickerBuilder struct{}

func (*weightedRRPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	subConns := make([]balancer.SubConn, 0, len(info.ReadySCs))
	weights := make([]int, 0, len(info.ReadySCs))
	total := 0

	for sc, scInfo := range info.ReadySCs {
		weight := extractWeight(scInfo.Address.Attributes)
		if weight <= 0 {
			weight = 1
		}
		subConns = append(subConns, sc)
		weights = append(weights, weight)
		total += weight
	}

	if total <= 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	return &weightedRRPicker{
		subConns: subConns,
		weights:  weights,
		total:    total,
	}
}

type weightedRRPicker struct {
	subConns []balancer.SubConn
	weights  []int
	total    int
	counter  uint64
}

func (p *weightedRRPicker) Pick(balancer.PickInfo) (balancer.PickResult, error) {
	if len(p.subConns) == 0 || p.total <= 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	idx := atomic.AddUint64(&p.counter, 1)
	slot := int(idx % uint64(p.total))

	sum := 0
	for i, weight := range p.weights {
		sum += weight
		if slot < sum {
			return balancer.PickResult{SubConn: p.subConns[i]}, nil
		}
	}

	return balancer.PickResult{SubConn: p.subConns[len(p.subConns)-1]}, nil
}

func extractWeight(attrs *attributes.Attributes) int {
	if attrs == nil {
		return 0
	}
	if v := attrs.Value("weight"); v != nil {
		switch val := v.(type) {
		case int:
			return val
		case int32:
			return int(val)
		case int64:
			return int(val)
		case float64:
			return int(val)
		case float32:
			return int(val)
		case string:
			if parsed, err := strconv.Atoi(val); err == nil {
				return parsed
			}
		}
	}
	return 0
}
