package rtmp

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gwuhaolin/livego/protocol/amf"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	reTcUrl = regexp.MustCompile(`rtmp\:\/\/([^\/\:]+)(\:([^\/]+))?\/([^\?]+)(\?vhost=([^\/]+))?`)
)

type Conn struct {
	// constant
	conn        net.Conn
	isClient    bool
	handshakeFn func() error // (*Conn).clientHandshake or serverHandshake
	ssMgr       *streamSourceMgr
	//pubMgr      *publisherMgr

	readWriter *readWriter
	config     *Config
	logger     *logrus.Logger

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
	streamKey   string
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

	bytesSent uint32
	//bytesSentReset uint32
	bytesRecv      uint32
	bytesRecvReset uint32
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

	logger := c.logger.WithFields(logrus.Fields{"event": "Serve Rtmp Conn"})
	logger.Tracef("local: %s, remote: %s, network: %s", c.LocalAddr().String(), c.RemoteAddr().String(), c.LocalAddr().Network())

	/*
		if err := c.SetDeadline(time.Now().Add(10 * time.Second)); err != nil { //TODO: timeout config
			_ = c.logger.Log("level", "ERROR", "event", "failed to set deadline")
		}
	*/

	logger = c.logger.WithFields(logrus.Fields{"event": "serverHandshake"})
	if err := c.Handshake(); err != nil {
		logger.Error(err)
		return
	}
	logger.Info("success")

	logger = c.logger.WithFields(logrus.Fields{"event": "handleCommandMessage"})
	if err := c.handleCommandMessage(); err != nil {
		logger.Error(err)
		return
	}
	logger.Info("success")

	logger = c.logger.WithFields(logrus.Fields{"event": "discover tcUrl"})
	if err := c.discoverTcUrl(); err != nil {
		logger.Error(err)
		return
	}
	c.streamKey = genStreamKey(c.domain, c.appName, c.streamName)
	logger.WithFields(logrus.Fields{"domain": c.domain, "app": c.appName, "stream": c.streamName, "args": c.args, "streamKey": c.streamKey}).Info("")

	if c.isPublisher { // publish
		logger = c.logger.WithFields(logrus.Fields{"event": "publish"})

		var ss *streamSource
		val, ok := c.ssMgr.streamMap.Load(c.streamKey)
		if !ok { //stream source not exists
			pub := newPublisher(c, c.streamKey)
			ss = newStreamSource(pub, c.streamKey, c.ssMgr)

			c.ssMgr.streamMap.Store(c.streamKey, ss) // save <streamKey, streamSource> pair
		} else {
			ss = val.(*streamSource)
			if ss.publisher != nil { // stream exists and is publishing
				logger.Error("stream is busy")
				return
			} else {
				ss.setPublisher(newPublisher(c, c.streamKey))
			}
		}

		defer ss.delPublisher()
		if err := ss.doPublishing(); err != nil {
			return
		}
	} else { //play
		logger = c.logger.WithFields(logrus.Fields{"event": "play"})

		val, ok := c.ssMgr.streamMap.Load(c.streamKey)
		if !ok {
			logger.Error("stream not exists")
			return
		}

		sub := newSubscriber(c, 1024) //TODO: avQueueSize use config's value
		ss := val.(*streamSource)
		if !ss.addSubscriber(sub) {
			logger.Error("already subscribe")
			return
		}

		defer ss.delSubscriber(sub)
		if err := ss.doPlaying(sub); err != nil {
			return
		}
	}
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
	basicHdrBuf := make([]byte, 3) // rtmp chunk basic header, at most 3 bytes
	logger := c.logger.WithFields(logrus.Fields{"event": "recv chunk stream"})

	for {
		cs, err := c.readChunkStream(basicHdrBuf)
		if err != nil {
			logger.Error(err)
			return err
		}
		logger.WithField("data", fmt.Sprintf("%#v", cs)).Info("")

		switch cs.MsgTypeID {
		case MsgAMF0CommandMessage, MsgAMF3CommandMessage:
			if err := c.decodeCommandMessage(cs); err != nil {
				logger.WithField("action", "decodeCommandMessage").Error(err)
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

func (c *Conn) decodeCommandMessage(cs *ChunkStream) error {
	if cs.MsgTypeID == MsgAMF3CommandMessage {
		cs.ChunkBody = cs.ChunkBody[1:]
	}

	r := bytes.NewReader(cs.ChunkBody)
	vs, err := c.amfDecoder.DecodeBatch(r, amf.Version(amf.AMF0))
	if err != nil && err != io.EOF {
		c.logger.WithField("event", "amf decode chunk body").Error(err)
		return err
	}
	c.logger.WithField("event", "amf decode chunk body").WithField("data", fmt.Sprintf("%#v", vs)).Trace("")

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
			c.logger.WithField("event", "decode Publish Msg").Info("success")
		case cmdPlay:
			if err := c.decodePlayCmdMessage(vs[1:]); err != nil {
				return err
			}
			if err := c.respPlayCmdMessage(cs); err != nil {
				return err
			}

			c.handleCommandMessageDone = true
			c.isPublisher = false
			c.logger.WithField("event", "decode Play Msg").Info("success")
		case cmdFCUnpublish, cmdDeleteStream:
		default:
			err := fmt.Errorf("unsupport command=%s", cmdStr)
			c.logger.WithField("event", "parse AMF command").Error(err)
			return err
		}
	}

	return nil
}

func (c *Conn) decodeConnectCmdMessage(vs []interface{}) error {
	for _, v := range vs {
		switch v := v.(type) {
		case string:
		case float64:
			id := int(v)
			if id != 1 {
				return fmt.Errorf("req error: %s", "id not 1 while connect")
			}
			c.transactionID = id
		case amf.Object:
			if app, ok := v["app"]; ok {
				c.appName = app.(string)
			}

			if flashVer, ok := v["flashVer"]; ok {
				c.flashVer = flashVer.(string)
			}

			if swfUrl, ok := v["swfUrl"]; ok {
				c.swfUrl = swfUrl.(string)
			}

			if tcUrl, ok := v["tcUrl"]; ok {
				c.tcUrl = tcUrl.(string)
			}

			if encoding, ok := v["objectEncoding"]; ok {
				c.objectEncoding = int(encoding.(float64))
			}
		}
	}

	c.logger.WithFields(logrus.Fields{
		"event": "parse connect command msg",
		"data": fmt.Sprintf("tid: %d, app: '%s', flashVer: '%s', swfUrl: '%s', tcUrl: '%s', objectEncoding: %d",
			c.transactionID, c.appName, c.flashVer, c.swfUrl, c.tcUrl, c.objectEncoding),
	}).Info("")

	return nil
}

func (c *Conn) respConnectCmdMessage(cs *ChunkStream) error {
	// WindowAcknowledgement Size
	respCs := NewProtolControlMessage(MsgWindowAcknowledgementSize, 4, c.localWindowAckSize)
	if err := c.writeChunStream(respCs); err != nil {
		c.logger.WithField("event", "Set WindowAckSize Message").Error(err)
		return err
	}
	c.logger.WithField("event", "Set WindowAckSize Message").Info("success")

	// Set Peer Bandwidth
	respCs = NewProtolControlMessage(MsgSetPeerBandwidth, 5, 2500000)
	respCs.ChunkBody[4] = 2
	if err := c.writeChunStream(respCs); err != nil {
		c.logger.WithField("event", "Set Peer Bandwidth").Error(err)
		return err
	}
	c.logger.WithField("event", "Set Peer Bandwidth").Info("success")

	// set chunk size
	respCs = NewProtolControlMessage(MsgSetChunkSize, 4, c.localChunksize)
	if err := c.writeChunStream(respCs); err != nil {
		c.logger.WithField("event", "Set Chunk Size").Error(err)
		return err
	}
	c.logger.WithField("event", "Set Chunk Size").Info("success")

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
		c.logger.WithField("event", "NetConnection.Connect.Success").Error(err)
		return err
	}
	c.logger.WithField("event", "NetConnection.Connect.Success").Info("success")

	return nil
}

func (c *Conn) decodeCreateStreamCmdMessage(vs []interface{}) error {
	for _, v := range vs {
		switch v := v.(type) {
		case string:
		case float64:
			c.transactionID = int(v)
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
	// set recorded
	cs1 := NewUserControlMessage(streamIsRecorded, 4)
	for i := 0; i < 4; i++ {
		cs1.ChunkBody[i+2] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	if err := c.writeChunStream(cs1); err != nil {
		return errors.Wrap(err, "send user control message streamIsRecorded")
	}

	// set begin
	cs2 := NewUserControlMessage(streamBegin, 4)
	for i := 0; i < 4; i++ {
		cs2.ChunkBody[i+2] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	if err := c.writeChunStream(cs2); err != nil {
		return errors.Wrap(err, "send user control message streamBegin")
	}

	// NetStream.Play.Resetstream
	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetStream.Play.Reset"
	event["description"] = "Playing and resetting stream."
	if err := c.writeMsg(cs.Csid, cs.MsgStreamID, "onStatus", 0, nil, event); err != nil {
		return errors.Wrap(err, "send NetStream.Play.Reset message")
	}

	// NetStream.Play.Start
	event["level"] = "status"
	event["code"] = "NetStream.Play.Start"
	event["description"] = "Started playing stream."
	if err := c.writeMsg(cs.Csid, cs.MsgStreamID, "onStatus", 0, nil, event); err != nil {
		return errors.Wrap(err, "send NetStream.Play.Start message")
	}

	// NetStream.Data.Start
	event["level"] = "status"
	event["code"] = "NetStream.Data.Start"
	event["description"] = "Started playing stream."
	if err := c.writeMsg(cs.Csid, cs.MsgStreamID, "onStatus", 0, nil, event); err != nil {
		return errors.Wrap(err, "send NetStream.Data.Start message")
	}

	// NetStream.Play.PublishNotify
	event["level"] = "status"
	event["code"] = "NetStream.Play.PublishNotify"
	event["description"] = "Started playing notify."
	if err := c.writeMsg(cs.Csid, cs.MsgStreamID, "onStatus", 0, nil, event); err != nil {
		return errors.Wrap(err, "send NetStream.Play.PublishNotify message")
	}

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
		switch v := v.(type) {
		case string:
			if k == 2 {
				c.streamName = v
			} else if k == 3 {
				if c.appName == "" {
					c.appName = v //has assigned very likely while decode connect command message
				}
			}
		case float64:
			c.transactionID = int(v)
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
			c.logger.WithField("event", "amf encode").Error(err)
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
		ChunkBody: cmdMsgBody,
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
