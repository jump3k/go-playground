package random

import (
	"math/rand"
	"testing"
	"time"
)

func TestRandomBalance(t *testing.T) {
	rand.Seed(time.Now().Unix())

	lr := RandomBalance{}

	_ = lr.Add("1.1.1.1")
	_ = lr.Add("2.2.2.2")

	node, err := lr.Get()
	if err != nil {
		t.Log(err)
		return
	}

	t.Log(node)
}
