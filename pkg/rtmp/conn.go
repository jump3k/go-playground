package rtmp

import (
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Conn struct {
	// constant
	conn        net.Conn
	isClient    bool
	handshakeFn func() error // (*Conn).clientHandshake or serverHandshake

	config *Config

	handshakeMutex  sync.Mutex
	HandshakeStatus uint32
	handshakeErr    error

	//rawInput  bytes.Buffer // raw input
	//input     bytes.Reader // application data waiting to be read, from rawInput.Next
	buffering bool
	sendBuf   []byte // a buffer of records waiting to be sent

	bytesSent int64
	//packetSent int64
}

func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *Conn) Close() error {
	//TODO
	return c.conn.Close()
}

func (c *Conn) Read(b []byte) (int, error) {
	if err := c.Handshake(); err != nil {
		return 0, err
	}

	//TODO
	return c.conn.Read(b)
}

func (c *Conn) Write(b []byte) (int, error) {
	return c.conn.Write(b)
}

func (c *Conn) Handshake() error {
	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()

	if err := c.handshakeErr; err != nil {
		return err
	}

	if c.handshakeComplete() {
		return nil
	}

	c.handshakeErr = c.handshakeFn()
	if c.handshakeErr == nil {
		c.HandshakeStatus++
	} else {
		c.flush()
	}

	if c.handshakeErr == nil && !c.handshakeComplete() {
		c.handshakeErr = errors.New("rtmp: internal error: handshake should be have had a result")
	}

	return c.handshakeErr
}

func (c *Conn) Serve() {
	log.Printf("start to handle rtmp conn, local: %s(%s), remote: %s(%s)\n",
		c.LocalAddr().String(), c.LocalAddr().Network(),
		c.RemoteAddr().String(), c.RemoteAddr().Network())

	if err := c.Handshake(); err != nil {
		return
	}
}

func (c *Conn) flush() (int, error) {
	if len(c.sendBuf) == 0 {
		return 0, nil
	}

	n, err := c.conn.Write(c.sendBuf)
	c.bytesSent += int64(n)
	c.sendBuf = nil
	c.buffering = false
	return n, err
}

func (c *Conn) ConnectionState() ConnectionState {
	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()
	return c.conncetionStateLocked()
}

func (c *Conn) conncetionStateLocked() ConnectionState {
	var state ConnectionState
	state.HandshakeComplete = c.handshakeComplete()

	return state
}

func (c *Conn) handshakeComplete() bool {
	return atomic.LoadUint32(&c.HandshakeStatus) == 1
}
