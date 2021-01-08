package routinepool

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestUnlimit(t *testing.T) {
	jobsCount := 100
	wg := sync.WaitGroup{}

	for i := 0; i < jobsCount; i++ {
		wg.Add(1)

		go func(i int) {
			fmt.Printf("hello %d!\n", i)
			time.Sleep(time.Second)
			wg.Done()
		}(i)

		fmt.Printf("index: %d, goroutine Num: %d \n", i, runtime.NumGoroutine())
	}

	wg.Wait()
	fmt.Println("done")
}

func TestLimit(t *testing.T) {
	jobCount := 100
	wg := sync.WaitGroup{}
	jobsChan := make(chan int, 3)

	poolCount := 3
	for i := 0; i < poolCount; i++ {
		go func() {
			for j := range jobsChan {
				fmt.Printf("hello %d\n", j)
				time.Sleep(time.Second)
				wg.Done()
			}
		}()
	}

	for i := 0; i < jobCount; i++ {
		jobsChan <- i
		wg.Add(1)
		fmt.Printf("index: %d, goroutine Num: %d \n", i, runtime.NumGoroutine())
	}

	wg.Wait()
	fmt.Println("done")
}
