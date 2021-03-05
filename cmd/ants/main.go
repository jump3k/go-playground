package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
)

func main() {
	commonPool()
	funcPool()
}

type taskArgs struct {
	input  int32
	output *int32
}

func funcPool() {
	var sum int32

	var wg sync.WaitGroup
	p, _ := ants.NewPoolWithFunc(10, func(args interface{}) {
		defer wg.Done()
		myFunc(args)
	})
	defer p.Release()

	for i := 0; i <= 100; i++ {
		wg.Add(1)

		args := &taskArgs{input: int32(i), output: &sum}
		_ = p.Invoke(args)
	}

	wg.Wait()
	fmt.Printf("running goroutines: %d\n", p.Running())
	fmt.Printf("finish all tasks, result is %d\n", sum)
}

func myFunc(param interface{}) {
	args := param.(*taskArgs)
	atomic.AddInt32(args.output, args.input)
}

func commonPool() {
	defer ants.Release()

	runTime := 1000

	var wg sync.WaitGroup
	syncCalculateSum := func() {
		defer wg.Done()
		demoFunc()
	}

	for i := 0; i < runTime; i++ {
		wg.Add(1)
		_ = ants.Submit(syncCalculateSum) // add task
	}

	wg.Wait()
	fmt.Printf("running goroutines: %d\n", ants.Running())
	fmt.Printf("finish all tasks.\n")
}

func demoFunc() {
	time.Sleep(10 * time.Millisecond)
	fmt.Println("Hello world!")
}
