package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"playground/pkg/rtmp"
)

func main() {
	go func() {
		_ = http.ListenAndServe(":6060", nil) //pprof
	}()

	go func() {
		if err := rtmp.ListenAndServe("tcp", ":1935", &rtmp.Config{}); err != nil {
			log.Fatal(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
