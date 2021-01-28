package rtmp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gwuhaolin/livego/protocol/amf"
)

var (
	reTcUrl = regexp.MustCompile(`rtmp\:\/\/([^\/\:]+)(\:([^\/]+))?\/([^\?]+)(\?vhost=([^\/]+))?`)
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

	transactionID int

	// client connect info
	appName        string
	flashVer       string
	swfUrl         string
	tcUrl          string
	objectEncoding int

	streamName  string
	domain      string
	args        string
	isPublisher bool

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

func (c *Conn) Serve() {
	defer c.Close()

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

	if err := c.discoverTcUrl(); err != nil {
		_ = c.logger.Log("level", "INFO", "event", "discover tcUrl", "error", err.Error())
		return
	}
	_ = c.logger.Log("level", "INFO", "event", "discover tcUrl", "data",
		fmt.Sprintf("domain: %s, app: %s, stream: %s, args: %s", c.domain, c.appName, c.streamName, c.args))
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

func (c *Conn) handleCommandMessage() error {
	for {
		cs, err := c.readChunkStream()
		if err != nil {
			_ = c.logger.Log("level", "ERROR", "event", "recv chunk stream", "error", err.Error())
			return err
		}
		_ = c.logger.Log("level", "INFO", "event", "recv chunk stream", "data", fmt.Sprintf("%#v", cs))

		//c.handleControlMessage(cs) // save remote chunksize and window ack size
		c.ack(cs.MsgLength)

		switch cs.MsgTypeID {
		case MsgSetChunkSize:
			c.remoteChunkSize = binary.BigEndian.Uint32(cs.ChunkData)
			_ = c.logger.Log("level", "INFO", "event", "save remoteChunkSize", "data", c.remoteChunkSize)
		case MsgWindowAcknowledgementSize:
			c.remoteWindowAckSize = binary.BigEndian.Uint32(cs.ChunkData)
			_ = c.logger.Log("level", "INFO", "event", "save remoteWindowAckSize", "data", c.remoteWindowAckSize)
		case MsgAMF0CommandMessage, MsgAMF3CommandMessage:
			if err := c.decodeCommandMessage(cs); err != nil {
				return err
			}
		}

		if c.handleCommandMessageDone {
			break
		}
	}

	return nil
}

func (c *Conn) discoverTcUrl() error {
	matches := reTcUrl.FindStringSubmatch(c.tcUrl)

	host := matches[1]  //ip addr or domain
	vhost := matches[6] //vhost=?

	c.domain = vhost
	if c.domain == "" {
		c.domain = host
	}

	if idx := strings.Index(c.appName, "?"); idx > 0 {
		c.appName = c.appName[:idx]
		c.appName = strings.Trim(c.appName, " ")
	}

	if idx := strings.Index(c.streamName, "?"); idx > 0 {
		c.args = c.streamName[idx+1:]
		c.streamName = c.streamName[:idx]

		c.args = strings.Trim(c.args, " ")
		c.streamName = strings.Trim(c.streamName, " ")
	}

	return nil
}

/*
func (c *Conn) handleControlMessage(cs *ChunkStream) {
	if cs.MsgTypeID == MsgSetChunkSize {
		c.remoteChunkSize = binary.BigEndian.Uint32(cs.ChunkData)
		_ = c.logger.Log("level", "INFO", "event", "save remoteChunkSize", "data", c.remoteChunkSize)
	} else if cs.MsgTypeID == MsgWindowAcknowledgementSize {
		c.remoteWindowAckSize = binary.BigEndian.Uint32(cs.ChunkData)
		_ = c.logger.Log("level", "INFO", "event", "save remoteWindowAckSize", "data", c.remoteWindowAckSize)
	}
}
*/

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

func (c *Conn) decodeCommandMessage(cs *ChunkStream) error {
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
			if err := c.decodeConnectCmdMessage(vs[1:]); err != nil {
				return err
			}
			if err := c.respConnectCmdMessage(cs); err != nil {
				return err
			}
		case cmdReleaseStream: // "releaseStream"
			_ = c.decodeReleaseStreamCmdMessage(vs[1:]) //do nothing
		case cmdFcpublish: // "FCPublish"
			_ = c.decodeFcPublishCmdMessage(vs[1:]) //do nothing
		case cmdCreateStream: // "createStream"
			if err := c.decodeCreateStreamCmdMessage(vs[1:]); err != nil {
				return err
			}
			if err := c.respCreateStreamCmdMessage(cs); err != nil {
				return err
			}
		case cmdPublish: // "publish"
			if err := c.decodePulishCmdMessage(vs[1:]); err != nil {
				return err
			}
			if err := c.respPulishCmdMessage(cs); err != nil {
				return err
			}

			c.handleCommandMessageDone = true
			c.isPublisher = true
			_ = c.logger.Log("level", "INFO", "event", "decode Publish Msg", "ret", "success")
		case cmdPlay:
			if err := c.decodePlayCmdMessage(vs[1:]); err != nil {
				return err
			}
			if err := c.respPlayCmdMessage(cs); err != nil {
				return err
			}

			c.handleCommandMessageDone = true
			c.isPublisher = false
			_ = c.logger.Log("level", "INFO", "event", "decode Play Msg", "ret", "success")
		case cmdFCUnpublish, cmdDeleteStream:
		default:
			err := fmt.Errorf("unsupport command=%s", cmdStr)
			_ = c.logger.Log("level", "ERROR", "event", "parse AMF command", "error", err.Error())
			return err
		}
	}

	return nil
}

func (c *Conn) decodeConnectCmdMessage(vs []interface{}) error {
	for _, v := range vs {
		switch v.(type) {
		case string:
		case float64:
			id := int(v.(float64))
			if id != 1 {
				return fmt.Errorf("req error: %s", "id not 1 while connect")
			}
			c.transactionID = id
		case amf.Object:
			objMap := v.(amf.Object)
			if app, ok := objMap["app"]; ok {
				c.appName = app.(string)
			}

			if flashVer, ok := objMap["flashVer"]; ok {
				c.flashVer = flashVer.(string)
			}

			if swfUrl, ok := objMap["swfUrl"]; ok {
				c.swfUrl = swfUrl.(string)
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
		"data", fmt.Sprintf("tid: %d, app: '%s', flashVer: '%s', swfUrl: '%s', tcUrl: '%s', objectEncoding: %d",
			c.transactionID, c.appName, c.flashVer, c.swfUrl, c.tcUrl, c.objectEncoding))

	return nil
}

func (c *Conn) respConnectCmdMessage(cs *ChunkStream) error {
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

func (c *Conn) decodeCreateStreamCmdMessage(vs []interface{}) error {
	for _, v := range vs {
		switch v.(type) {
		case string:
		case float64:
			c.transactionID = int(v.(float64))
		case amf.Object:
		}
	}
	return nil
}

func (c *Conn) respCreateStreamCmdMessage(cs *ChunkStream) error {
	return c.writeMsg(cs.Csid, cs.MsgStreamID, "_result", c.transactionID, nil, 1 /*streamID*/)
}

func (c *Conn) decodePulishCmdMessage(vs []interface{}) error {
	return c.publishOrPlay(vs)
}

func (c *Conn) respPulishCmdMessage(cs *ChunkStream) error {
	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetStream.Publish.Start"
	event["description"] = "Start publising."

	return c.writeMsg(cs.Csid, cs.MsgStreamID, "onStatus", 0, nil, event)
}

func (c *Conn) decodePlayCmdMessage(vs []interface{}) error {
	return c.publishOrPlay(vs)
}

func (c *Conn) respPlayCmdMessage(cs *ChunkStream) error {
	//TODO:
	return nil
}

func (c *Conn) decodeFcPublishCmdMessage(vs interface{}) error {
	return nil
}

/*
func (c *Conn) respFcPublishCmdMessage(cs *ChunkStream) error {
	return nil
}
*/

func (c *Conn) decodeReleaseStreamCmdMessage(vs interface{}) error {
	return nil
}

/*
func (c *Conn) respReleaseStreamCmdMessage(cs *ChunkStream) error {
	return nil
}
*/

func (c *Conn) publishOrPlay(vs []interface{}) error {
	for k, v := range vs {
		switch v.(type) {
		case string:
			if k == 2 {
				c.streamName = v.(string)
			} else if k == 3 {
				if c.appName == "" {
					c.appName = v.(string) //has assigned very likely while decode connect command message
				}
			}
		case float64:
			c.transactionID = int(v.(float64))
		case amf.Object:
		}
	}

	return nil
}

// send MsgAMF0CommandMessage msg
func (c *Conn) writeMsg(csid, streamID uint32, args ...interface{}) error {
	buffer := bytes.NewBuffer([]byte{})
	for _, v := range args {
		if _, err := c.amfEncoder.Encode(buffer, v, amf.AMF0); err != nil {
			_ = c.logger.Log("level", "ERROR", "event", "amf encode", "error", err.Error())
			return err
		}
	}
	cmdMsgBody := buffer.Bytes()

	cs := ChunkStream{
		ChunkHeader: ChunkHeader{
			ChunkBasicHeader: ChunkBasicHeader{
				Fmt:  0,
				Csid: csid,
			},
			ChunkMessageHeader: ChunkMessageHeader{
				TimeStamp:   0,
				MsgLength:   uint32(len(cmdMsgBody)),
				MsgTypeID:   MsgAMF0CommandMessage,
				MsgStreamID: streamID,
			},
		},
		ChunkData: cmdMsgBody,
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
