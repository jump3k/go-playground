package main

import (
	"fmt"
	"log"
	"net/rpc"
)

const HelloServiceName = "xxx/pkg.HelloService"

type HelloServerClient struct {
	*rpc.Client
}

func DialHelloService(network, address string) (*HelloServerClient, error) {
	c, err := rpc.Dial(network, address)
	if err != nil {
		return nil, err
	}

	return &HelloServerClient{Client: c}, nil
}

func (p *HelloServerClient) Hello(request string, reply *string) error {
	return p.Client.Call(HelloServiceName+".Hello", request, reply)
}

func main() {
	/*

		client, err := rpc.Dial("tcp", "localhost:1234")
		if err != nil {
			log.Fatal(err)
		}

		var reply string
		err = client.Call(HelloServiceName+".Hello", "1.1.1.1", &reply)
		if err != nil {
			log.Fatal(err)
		}
	*/
	client, err := DialHelloService("tcp", "127.0.0.1:1234")
	if err != nil {
		log.Fatal(err)
	}

	for i := 0; i < 1; i++ {
		var reply string
		err = client.Hello("xly", &reply)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(reply)
	}
}
