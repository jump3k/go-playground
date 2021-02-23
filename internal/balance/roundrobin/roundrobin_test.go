package roundrobin

import "testing"

func TestRoundRobin(t *testing.T) {
	rr := RoundRobinBalance{}

	if err := rr.Add("1.1.1.1", "2.2.2.2", "3.3.3.3"); err != nil {
		t.Log(err)
		return
	}

	for i := 1; i <= 4; i++ {
		node, err := rr.Get()
		if err != nil {
			t.Log(err)
			return
		}

		t.Log(node)
	}
}
