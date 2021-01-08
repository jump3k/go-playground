package main

import (
	"playground/internal/srshookserver"
)

func main() {
	if err := srshookserver.NewSrsHookServer(":6666").Run(); err != nil {
		return
	}
}
