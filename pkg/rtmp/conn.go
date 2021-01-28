package rtmp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gwuhaolin/livego/protocol/amf"
)

type Conn struct {
	// constant
	conn        net.Conn
	isClient    bool
	handshakeFn func() error // (*Conn).clientHandshake or serverHandshake

	config *Config
	logger log.Logger

	handshakeMutex  sync.Mutex
	HandshakeStatus uint32
	handshakeErr    error

	transactionID            int
	app                      string
	flashVer                 string
	tcUrl                    string
	objectEncoding           int
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
	_ = c.logger.Log("level", "INFO",
		"event", "start to handle rtmp conn",
		"local", c.LocalAddr().String()+" ("+c.LocalAddr().Network()+")",
		"remote", c.RemoteAddr().String()+" ("+c.RemoteAddr().Network()+")",
	)

	if err := c.SetDeadline(time.Now().Add(10 * time.Second)); err != nil { //TODO: timeout config
		_ = c.logger.Log("level", "ERROR", "event", "failed to set deadline")
	}

	if err := c.Handshake(); err != nil {
		_ = c.logger.Log("level", "ERROR", "event", "serverHandshake", "error", err.Error())
		return
	}
	_ = c.logger.Log("level", "INFO", "event", "serverHandshake", "ret", "success")

	if err := c.handleCommandMessage(); err != nil {
		_ = c.logger.Log("level", "ERROR", "event", "handleCommandMessage", "error", err.Error())
		return
	}
	_ = c.logger.Log("level", "INFO", "event", "handleCommandMessage", "ret", "success")
}

func (c *Conn) handleCommandMessage() error {
	for {
		cs, err := c.readChunkStream()
		if err != nil {
			_ = c.logger.Log("level", "ERROR", "event", "recv chunk stream", "error", err.Error())
			return err
		}
		_ = c.logger.Log("level", "INFO", "event", "recv chunk stream", "data", fmt.Sprintf("%#v", cs))

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
	_ = c.logger.Log("level", "INFO", "event", "save remote chunkSize/WinAckSize", "data",
		fmt.Sprintf("remoteChunkSize: %d, remoteWinAckSize: %d", c.remoteChunkSize, c.remoteWindowAckSize))
}

func (c *Conn) ack(size uint32) {
	c.bytesSent += size
	if c.bytesSent >= 1<<32-1 {
		c.bytesSent = 0
		c.bytesSentReset++
	}

	c.ackSeqNumber += size
	if c.ackSeqNumber >= c.remoteWindowAckSize { //超过窗口通告大小，回复ACK
		cs := newChunkStream().asControlMessage(MsgAcknowledgement, 4, c.ackSeqNumber)
		if err := c.writeChunStream(cs); err != nil {
			_ = c.logger.Log("level", "ERROR", "event", "send Ack", "error", err.Error())
		}
		_ = c.logger.Log("level", "INFO", "event", "send Ack", "ret", "success")

		c.ackSeqNumber = 0
	}
}

func (c *Conn) responseCommandMessage(cs *ChunkStream) error {
	if cs.MsgTypeID == MsgAMF3CommandMessage {
		cs.ChunkData = cs.ChunkData[1:]
	}

	r := bytes.NewReader(cs.ChunkData)
	vs, err := c.amfDecoder.DecodeBatch(r, amf.Version(amf.AMF0))
	if err != nil && err != io.EOF {
		_ = c.logger.Log("level", "ERROR", "event", "amf decode chunk body", "error", err.Error())
		return err
	}
	_ = c.logger.Log("level", "INFO", "event", "amf decode chunk body", "data", fmt.Sprintf("%#v", vs))

	if cmdStr, ok := vs[0].(string); ok {
		switch cmdStr {
		case cmdConnect: // "connect"
			if err := c.handleCmdConnectMessage(vs[1:]); err != nil {
				return err
			}
			if err := c.respCmdConnectMessage(cs); err != nil {
				return err
			}
		case cmdCreateStream:
			//TODO:
		case cmdPublish:
			//TODO:
		case cmdPlay:
			//TODO:
		case cmdFcpublish:
			//TODO:
		case cmdReleaseStream:
			//TODO:
		case cmdFCUnpublish, cmdDeleteStream:
			//TODO:
		default:
			err := fmt.Errorf("unsupport command=%s", cmdStr)
			_ = c.logger.Log("level", "ERROR", "event", "parse AMF command", "error", err.Error())
			return err
		}
	}

	return nil
}

func (c *Conn) handleCmdConnectMessage(vs []interface{}) error {
	for _, v := range vs {
		switch v.(type) {
		case string, float64:
			id := int(v.(float64))
			if id != 1 {
				return fmt.Errorf("req error: %s", "id not 1 while connect")
			}
			c.transactionID = id
		case amf.Object:
			objMap := v.(amf.Object)
			if app, ok := objMap["app"]; ok {
				c.app = app.(string)
			}

			if flashVer, ok := objMap["flashVer"]; ok {
				c.flashVer = flashVer.(string)
			}

			if tcUrl, ok := objMap["tcUrl"]; ok {
				c.tcUrl = tcUrl.(string)
			}

			if encoding, ok := objMap["objectEncoding"]; ok {
				c.objectEncoding = int(encoding.(float64))
			}
		}
	}

	_ = c.logger.Log("level", "INFO", "event", "parse connect command msg",
		"data", fmt.Sprintf("tid: %d, app: '%s', flashVer: '%s', tcUrl: '%s', objectEncoding: %d",
			c.transactionID, c.app, c.flashVer, c.tcUrl, c.objectEncoding))

	return nil
}

func (c *Conn) respCmdConnectMessage(cs *ChunkStream) error {
	// WindowAcknowledgement Size
	respCs := newChunkStream().asControlMessage(MsgWindowAcknowledgementSize, 4, c.localWindowAckSize)
	if err := c.writeChunStream(respCs); err != nil {
		_ = c.logger.Log("level", "ERROR", "event", "Set WindowAckSize Message", "error", err.Error())
		return err
	}
	_ = c.logger.Log("level", "INFO", "event", "Send WindowAckSize Message", "ret", "success")

	// Set Peer Bandwidth
	respCs = newChunkStream().asControlMessage(MsgSetPeerBandwidth, 5, 2500000)
	respCs.ChunkData[4] = 2
	if err := c.writeChunStream(respCs); err != nil {
		_ = c.logger.Log("level", "ERROR", "event", "Set Peer Bandwidth", "error", err.Error())
		return err
	}
	_ = c.logger.Log("level", "INFO", "event", "Set Peer Bandwidth", "ret", "success")

	// set chunk size
	respCs = newChunkStream().asControlMessage(MsgSetChunkSize, 4, c.localChunksize)
	if err := c.writeChunStream(respCs); err != nil {
		_ = c.logger.Log("level", "ERROR", "event", "Set Chunk Size", "error", err.Error())
		return err
	}
	_ = c.logger.Log("level", "INFO", "event", "Set Chunk Size", "ret", "success")

	// NetConnection.Connect.Success
	resp := make(amf.Object)
	resp["fmsVer"] = "FMS/3,0,1,123"
	resp["capabilities"] = 31

	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetConnection.Connect.Success"
	event["description"] = "Connection succeeded."
	event["objectEncoding"] = c.objectEncoding
	if err := c.writeMsg(cs.Csid, cs.MsgStreamID, "_result", c.transactionID, resp, event); err != nil {
		_ = c.logger.Log("level", "ERROR", "event", "NetConnection.Connect.Success", "error", err.Error())
		return err
	}
	_ = c.logger.Log("level", "INFO", "event", "NetConnection.Connect.Success", "ret", "success")

	return nil
}

// send MsgAMF0CommandMessage msg
func (c *Conn) writeMsg(csid, streamID uint32, args ...interface{}) error {
	var bytesSend []byte
	for _, v := range args {
		if _, err := c.amfEncoder.Encode(bytes.NewBuffer(bytesSend), v, amf.AMF0); err != nil {
			_ = c.logger.Log("level", "ERROR", "event", "amf encode", "error", err.Error())
			return err
		}
	}

	cs := ChunkStream{
		ChunkHeader: ChunkHeader{
			ChunkBasicHeader: ChunkBasicHeader{
				Fmt:  0,
				Csid: csid,
			},
			ChunkMessageHeader: ChunkMessageHeader{
				TimeStamp:   0,
				MsgLength:   uint32(len(bytesSend)),
				MsgTypeID:   MsgAMF0CommandMessage,
				MsgStreamID: streamID,
			},
		},
		ChunkData: bytesSend,
	}

	if err := c.writeChunStream(&cs); err != nil {
		return err
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
