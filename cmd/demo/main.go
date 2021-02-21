package main

import (
	"fmt"
	"net/url"
)

func main() {
	tcUrl := "rtmp://push.xly.net:1935/live"

	u, err := url.Parse(tcUrl)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("u: %#v", u)
}
