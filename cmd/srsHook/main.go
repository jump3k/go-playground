package main

import (
	"playground/internal/srshookserver"
)

func main() {
	if err := srshookserver.New(":6666", "gin", "release").Run(); err != nil {
		return
	}
}
