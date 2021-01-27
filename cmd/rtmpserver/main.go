package main

import (
	"log"
	"playground/pkg/rtmp"
)

func main() {
	if err := rtmp.ListenAndServe("tcp", ":1935", &rtmp.Config{}); err != nil {
		log.Fatal(err)
	}
}
