package weightroundrobin

import "testing"

func TestWRR(t *testing.T) {
	wrr := &WeightRoundRobinBalance{}

	_ = wrr.Add("1.1.1.1", "1")
	_ = wrr.Add("2.2.2.2", "2")
	_ = wrr.Add("3.3.3.3", "1")

	for i := 0; i < 8; i++ {
		node, _ := wrr.Get()
		t.Log(node)
	}
}
