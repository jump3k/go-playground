package random

import (
	"errors"
	"math/rand"
)

// list store all the node
type RandomBalance struct {
	curIdx   int
	allNodes []string
}

// add node
func (r *RandomBalance) Add(params ...string) error {
	if len(params) == 0 {
		return errors.New("param len 1 at least")
	}

	r.allNodes = append(r.allNodes, params[0])

	return nil
}

// get node
func (r *RandomBalance) Get(...string) (string, error) {
	if len(r.allNodes) == 0 {
		return "", errors.New("alloNodes is empty")
	}

	r.curIdx = rand.Intn(len(r.allNodes))
	return r.allNodes[r.curIdx], nil
}
