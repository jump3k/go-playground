package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/nats-io/nats.go"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	var nc *nats.Conn
	cd := &customDialer{
		ctx:             ctx,
		nc:              nc,
		connectTimeout:  10 * time.Second,
		connectTimeWait: 1 * time.Second,
	}

	opts := []nats.Option{
		nats.SetCustomDialer(cd),
		nats.ReconnectWait(2 * time.Second),
		nats.ReconnectHandler(func(c *nats.Conn) {
			log.Println("Reconnected to", c.ConnectedUrl())
		}),
		nats.DisconnectHandler(func(c *nats.Conn) {
			log.Println("Disconnected from NATS")
		}),
		nats.ClosedHandler(func(c *nats.Conn) {
			log.Println("NATS connection is closed.")
		}),
		//nats.NoReconnect(),
	}

	go func() {
		nc, err = nats.Connect("127.0.0.1:4222", opts...)
	}()

waitForEstablishedConnection:
	for {
		if err != nil {
			log.Fatal(err)
		}

		select {
		case <-ctx.Done():
			break waitForEstablishedConnection
		default:
		}

		if nc == nil || !nc.IsConnected() {
			log.Println("Connection not ready")
			time.Sleep(200 * time.Millisecond)
			continue
		}
		break waitForEstablishedConnection
	}

	if ctx.Err() != nil {
		log.Fatal(ctx.Err())
	}

	for {
		if nc.IsClosed() {
			break
		}

		if err := nc.Publish("hello", []byte("world")); err != nil {
			log.Println(err)
			time.Sleep(1 * time.Second)
			continue
		}
		log.Println("Published message")
		time.Sleep(600 * time.Second)
	}

	if err := nc.Drain(); err != nil {
		log.Println(err)
	}
	log.Println("Disconnected")
}

type customDialer struct {
	ctx             context.Context
	nc              *nats.Conn
	connectTimeout  time.Duration //拨号失败超时时间
	connectTimeWait time.Duration //拨号失败时重试等待时间
}

func (cd *customDialer) Dial(network, address string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(cd.ctx, cd.connectTimeout)
	defer cancel()

	for {
		log.Println("Attempting to connect to", address)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		select {
		case <-cd.ctx.Done():
			return nil, cd.ctx.Err()
		default:
			d := &net.Dialer{}
			if conn, err := d.DialContext(ctx, network, address); err == nil {
				log.Println("Connected to NATS successfully")
				return conn, nil
			} else {
				time.Sleep(cd.connectTimeWait)
			}
		}
	}
}
