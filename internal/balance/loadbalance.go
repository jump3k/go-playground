package balance

import (
	"playground/internal/balance/consitenthash"
	"playground/internal/balance/random"
	"playground/internal/balance/roundrobin"
	"playground/internal/balance/weightroundrobin"
)

type LoadBalance interface {
	Add(...string) error
	Get(...string) (string, error)
}

const (
	Random = iota
	RoundRobin
	WeightRoundRobin
	ConsistentHash
)

func NewLoadBalance(lbType int) LoadBalance {
	switch lbType {
	case Random:
		return new(random.RandomBalance)
	case RoundRobin:
		return new(roundrobin.RoundRobinBalance)
	case WeightRoundRobin:
		return new(weightroundrobin.WeightRoundRobinBalance)
	case ConsistentHash:
		return consitenthash.NewConsistentHash(nil)
	default:
		return new(roundrobin.RoundRobinBalance)
	}
}
