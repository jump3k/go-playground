package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"syscall"

	"github.com/google/vectorio"
)

func main() {
	f, err := ioutil.TempFile("", "vectorio")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	data1 := []byte("foobarbaz")
	data2 := []byte("foobazbar")

	w, _ := vectorio.NewBufferedWritev(f)
	_, _ = w.Write(data2)
	_, _ = w.WriteIovec(syscall.Iovec{Base: &data1[0], Len: 9})
	nw, err := w.Flush()

	if err != nil {
		fmt.Printf("Flush threw error: %s", err)
	}

	if nw == 9*2 {
		fmt.Println("Wrote", nw, "bytes to file!")
	} else {
		fmt.Println("did not write 9 * 2 bytes, wrote ", nw)
	}

	buf := make([]byte, 18)
	_, _ = f.Seek(0, 0)
	if _, err := f.Read(buf); err != nil {
		if err != io.EOF {
			panic(err)
		}
	}

	fmt.Println(string(buf))
}
