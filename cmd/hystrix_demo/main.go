package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/afex/hystrix-go/hystrix"
)

func main() {
	hystrixStreamHandler := hystrix.NewStreamHandler()
	hystrixStreamHandler.Start()
	go http.ListenAndServe(net.JoinHostPort("", "81"), hystrixStreamHandler)

	go syncDemo()

	for i := 0; i < 50; i++ {
		go asyncDemo()
	}
	go customDemo()

	select {}
}

func syncDemo() {
	for {
		_ = hystrix.Do("req_google", func() error {
			_, err := (&http.Client{Timeout: 1 * time.Second}).Get("http://www.google.com")
			//fmt.Println(resp)
			return err
		}, func(err error) error {
			fmt.Printf("[sync] server down, error: %v\n", err)
			return nil
		})
		time.Sleep(50 * time.Millisecond)
	}
}

func asyncDemo() {
	for {
		output := make(chan struct{}, 1)
		errors := hystrix.Go("my_command", func() error {
			_, err := (&http.Client{Timeout: 1 * time.Second}).Get("http://www.google.com")

			output <- struct{}{}
			return err
		}, nil)

		select {
		case <-output:
			// success
		case err := <-errors:
			// failure
			fmt.Printf("[async] error: %v\n", err)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func customDemo() {
	name := "custom"
	hystrix.ConfigureCommand(name, hystrix.CommandConfig{
		Timeout:                int(time.Second * 3), //执行任务的超时时间（默认1000ms）
		MaxConcurrentRequests:  100,                  //最大并发量(默认10)
		SleepWindow:            int(time.Second * 5), //熔断后，过多久去重新尝试服务是否恢复
		RequestVolumeThreshold: 30,                   //一个统计窗口10秒内请求数量，超过才判断是否开启熔断(默认20)
		ErrorPercentThreshold:  50,                   //错误百分比，请求数超过RequestVolumeThreshold并且错误百分比达到该值，则启动熔断(默认50)
	})

	err := hystrix.DoC(context.Background(), name, func(ctx context.Context) error {
		_, err := (&http.Client{Timeout: time.Second}).Get("http://www.baidu.com/")
		if err != nil {
			return err
		}

		fmt.Println("succ")
		return err
	}, func(i context.Context, e error) error {
		fmt.Println("failed")
		return e
	})

	fmt.Println(err)
}
