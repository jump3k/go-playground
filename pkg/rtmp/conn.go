package rtmp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gwuhaolin/livego/protocol/amf"
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

	handleCommandMessageDone bool

	chunks map[uint32]*ChunkStream //<CSID, ChunkStream>

	amfDecoder *amf.Decoder
	amfEncoder *amf.Encoder

	localChunksize      uint32 // local chunk size
	localWindowAckSize  uint32 //local window ack size
	remoteChunkSize     uint32 //peer chunk size
	remoteWindowAckSize uint32 // peer window ack size
	ackSeqNumber        uint32 // window ack sequence number

	//rawInput  bytes.Buffer // raw input
	//input     bytes.Reader // application data waiting to be read, from rawInput.Next
	buffering bool
	sendBuf   []byte // a buffer of records waiting to be sent

	bytesSent      uint32
	bytesSentReset uint32
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
	return c.conn.Close()
}

func (c *Conn) Read(b []byte) (int, error) {
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

	if err := c.SetDeadline(time.Now().Add(10 * time.Second)); err != nil { //TODO: timeout config
		log.Println("failed to set deadline")
	}

	if err := c.Handshake(); err != nil {
		log.Printf("serverHandshake error: %v", err)
		return
	}
	log.Println("serverHandshake success.")

	if err := c.handleCommandMessage(); err != nil {
		log.Printf("handleCommandMessage error: %v", err)
		return
	}
	log.Println("handleCommandMessage success.")
}

func (c *Conn) handleCommandMessage() error {
	for {
		cs, err := c.ReadChunkStream() //read one chunk stream data fully
		if err != nil {
			return err
		}

		c.handleControlMessage(cs) // save remote chunksize and window ack size
		c.ack(cs.MsgLength)

		switch cs.MsgTypeID {
		case MsgAMF0CommandMessage, MsgAMF3CommandMessage:
			if err := c.responseCommandMessage(cs); err != nil {
				return err
			}
		}

		if c.handleCommandMessageDone {
			break
		}
	}

	return nil
}

func (c *Conn) handleControlMessage(cs *ChunkStream) {
	if cs.MsgTypeID == MsgSetChunkSize {
		c.remoteChunkSize = binary.BigEndian.Uint32(cs.ChunkData)
	} else if cs.MsgTypeID == MsgWindowAcknowledgementSize {
		c.remoteWindowAckSize = binary.BigEndian.Uint32(cs.ChunkData)
	}
}

func (c *Conn) ack(size uint32) {
	c.bytesSent += size
	if c.bytesSent >= 1<<32-1 {
		c.bytesSent = 0
		c.bytesSentReset++
	}

	c.ackSeqNumber += size
	if c.ackSeqNumber >= c.remoteWindowAckSize { //超过窗口通告大小，回复ACK
		cs := NewChunkStream().SetControlMessage(MsgAcknowledgement, 4, c.ackSeqNumber)
		if err := c.writeChunStream(cs); err != nil {
			log.Printf("send Acknowledgement message error: %v", err)
		}
		c.ackSeqNumber = 0
	}
}

func (c *Conn) responseCommandMessage(cs *ChunkStream) error {
	if cs.MsgTypeID == MsgAMF3CommandMessage {
		cs.ChunkData = cs.ChunkData[1:]
	}

	r := bytes.NewReader(cs.ChunkData)
	reqCs, err := c.amfDecoder.DecodeBatch(r, amf.Version(amf.AMF0))
	if err != nil && err != io.EOF {
		return err
	}
	log.Printf("rtmp request chunkStream: %#v", reqCs)

	return nil
}

func (c *Conn) writeChunStream(cs *ChunkStream) error {
	switch cs.MsgTypeID {
	case MsgAudioMessage:
		cs.Csid = 4
	case MsgVideoMessage, MsgAMF3DataMessage, MSGAMF0DataMessage:
		cs.Csid = 6
	}

	totalLen := uint32(0)
	numChunks := (cs.MsgLength / c.localChunksize) // split by local chunk size
	for i := uint32(0); i < numChunks; i++ {
		if totalLen == cs.MsgLength {
			break
		}

		if i == 0 {
			cs.Fmt = 0
		} else {
			cs.Fmt = 3
		}

		if err := c.writeHeader(cs); err != nil {
			return err
		}

		inc := c.localChunksize
		start := i * c.localChunksize

		leftLen := uint32(len(cs.ChunkData)) - start
		if leftLen < c.localChunksize {
			inc = leftLen
		}
		totalLen += inc

		buf := cs.ChunkData[start : start+inc]
		if _, err := c.Write(buf); err != nil {
			return err
		}
	}

	return nil
}

func (c *Conn) writeHeader(cs *ChunkStream) error {
	// basic header
	h := cs.Fmt << 6
	switch {
	case cs.Csid < 64:
		h |= uint8(cs.Csid)
		//TODO
	}

	return nil
}

func (c *Conn) flush() (int, error) {
	if len(c.sendBuf) == 0 {
		return 0, nil
	}

	n, err := c.conn.Write(c.sendBuf)
	c.bytesSent += uint32(n)
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
