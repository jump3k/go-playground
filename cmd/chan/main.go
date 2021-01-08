package main

import (
	"fmt"
	"time"
)

func main() {
	c := make(chan int, 1) //no cache

	go func() {
		c <- 3 + 4
	}()

	time.Sleep(time.Millisecond)
	close(c)

	for {
		if i, ok := <-c; !ok {
			fmt.Println("chan closed")
			break
		} else {
			fmt.Println(i)
		}
	}
}
