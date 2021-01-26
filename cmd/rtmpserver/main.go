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
	log.Printf("rtmp listen addr: %s(%s)", l.Addr().String(), l.Addr().Network())

	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}

		//fmt.Println(conn.RemoteAddr().String())
		go conn.(*rtmp.Conn).Serve()
	}
}
