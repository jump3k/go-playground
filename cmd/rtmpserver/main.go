package main

import (
	"log"
	"playground/pkg/rtmp"
)

func main() {
	l, err := rtmp.Listen("tcp", ":1935", &rtmp.Config{})
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}

		//fmt.Println(conn.RemoteAddr().String())
		go conn.(*rtmp.Conn).Serve()
	}
}
