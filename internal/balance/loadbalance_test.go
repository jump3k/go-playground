package balance

import (
	"math/rand"
	"playground/internal/balance/consitenthash"
	"strconv"
	"testing"
	"time"
)

func TestRandomBalance(t *testing.T) {
	lb := NewLoadBalance(Random)

	_ = lb.Add("1.1.1.1")
	_ = lb.Add("2.2.2.2")
	_ = lb.Add("3.3.3.3")

	rand.Seed(time.Now().Unix())

	for i := 0; i < 6; i++ {
		node, _ := lb.Get()
		t.Log(node)
	}
}

func TestRoundRobin(t *testing.T) {
	lb := NewLoadBalance(RoundRobin)

	_ = lb.Add("1.1.1.1")
	_ = lb.Add("2.2.2.2")
	_ = lb.Add("3.3.3.3")

	for i := 0; i < 6; i++ {
		node, _ := lb.Get()
		t.Log(node)
	}
}

func TestWRR(t *testing.T) {
	lb := NewLoadBalance(WeightRoundRobin)

	_ = lb.Add("1.1.1.1", "1")
	_ = lb.Add("2.2.2.2", "2")
	_ = lb.Add("3.3.3.3", "3")

	for i := 0; i < 6; i++ {
		node, _ := lb.Get()
		t.Log(node)
	}
}

func TestCHash(t *testing.T) {
	lb := NewLoadBalance(ConsistentHash)

	_ = lb.Add("1.1.1.1", "160")
	_ = lb.Add("2.2.2.2", "160")
	_ = lb.Add("3.3.3.3", "160")

	t.Logf("hashRingSize: %d", lb.(*consitenthash.ConsistentHashLoadBalance).GetHashRingSize())

	for i := 0; i < 6; i++ {
		name := "beijing" + "_" + strconv.Itoa(i)
		node, _ := lb.Get(name)
		t.Log(node)
	}
}
