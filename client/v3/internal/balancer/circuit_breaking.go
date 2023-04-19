package balancer

import (
	"math/rand"
	"sync/atomic"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
)

const Name = "circuit_breaking"

func newCircuitBreakingBuilder() balancer.Builder {
	return base.NewBalancerBuilder(Name, &circuitBreakingBuilder{}, base.Config{HealthCheck: true})
}

func init() {
	balancer.Register(newCircuitBreakingBuilder())
}

type circuitBreakingBuilder struct{}

func (c circuitBreakingBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	scs := make([]balancer.SubConn, 0, len(info.ReadySCs))
	for sc := range info.ReadySCs {
		scs = append(scs, sc)
	}
	return &circuitBreakingPicker{
		subConns: scs,
		next:     uint32(rand.Intn(len(scs))),
	}
}

type circuitBreakingPicker struct {
	subConns []balancer.SubConn
	next     uint32
}

func (p *circuitBreakingPicker) Done(info balancer.DoneInfo) {
	// we're missing:
	// * which connection was picked
	// * what its result was in terms of latency
	// * maybe need to incooperate server load info (not backward compatible)
}

func (p *circuitBreakingPicker) Pick(balancer.PickInfo) (balancer.PickResult, error) {
	subConnsLen := uint32(len(p.subConns))
	nextIndex := atomic.AddUint32(&p.next, 1)

	sc := p.subConns[nextIndex%subConnsLen]
	return balancer.PickResult{SubConn: sc, Done: p.Done}, nil
}
