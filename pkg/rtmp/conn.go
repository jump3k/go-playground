package rtmp

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gwuhaolin/livego/protocol/amf"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Conn struct {
	// constant
	conn       net.Conn
	readWriter *readWriter // reader & writer with buffer
	isClient   bool

	// config and logger pointer
	config *Config
	logger *logrus.Logger

	// rtmp handshake
	handshakeFn     func() error // (*Conn).clientHandshake or serverHandshake
	handshakeMutex  sync.Mutex
	HandshakeStatus uint32
	handshakeErr    error

	// handle command message
	transactionID            int
	amfDecoder               *amf.Decoder
	amfEncoder               *amf.Encoder
	handleCommandMessageDone bool

	// client connect info
	appName        string
	flashVer       string
	swfUrl         string
	tcUrl          string
	objectEncoding int

	// parse tcUrl result
	host      string
	port      int
	vhost     string
	rawQuery  string
	urlValues url.Values

	// client role and associate with stream source manager
	isPublisher bool             // true: publish  false: play
	streamName  string           // set while publish/play command
	ssMgr       *streamSourceMgr // stream source manager pointer
	streamKey   string           // generate by func genStreamKey

	basicHdrBuf []byte                  //rtmp chunk basic header, at most 3 bytes
	chunks      map[uint32]*ChunkStream //<CSID, ChunkStream>

	localChunksize      uint32 // local chunk size
	localWindowAckSize  uint32 // local window ack size
	remoteChunkSize     uint32 // peer chunk size
	remoteWindowAckSize uint32 // peer window ack size
	ackSeqNumber        uint32 // window ack sequence number

	bytesRecv      uint32
	bytesRecvReset uint32
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
	logger.Trace("success")

	logger = c.logger.WithFields(logrus.Fields{"event": "handleCommandMessage"})
	c.basicHdrBuf = make([]byte, 3)
	if err := c.handleCommandMessage(); err != nil {
		logger.Error(err)
		return
	}
	logger.Trace("success")

	logger = c.logger.WithFields(logrus.Fields{"event": "discover tcUrl"})
	if err := c.discoverTcUrl(); err != nil {
		logger.Error(err)
		return
	}
	c.streamKey = genStreamKey(c.vhost, c.appName, c.streamName)
	logger.WithFields(logrus.Fields{"vhost": c.vhost, "app": c.appName, "stream": c.streamName, "rawQuery": c.rawQuery, "streamKey": c.streamKey}).Trace("")

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
		c.readWriter.Flush()
	}

	if c.handshakeErr == nil && !c.handshakeComplete() {
		c.handshakeErr = errors.New("rtmp: internal error: handshake should be have had a result")
	}

	return c.handshakeErr
}

func (c *Conn) handleCommandMessage() error {
	logger := c.logger.WithFields(logrus.Fields{"event": "recv chunk stream"})

	for {
		cs, err := c.readChunkStream(c.basicHdrBuf)
		if err != nil {
			logger.Error(err)
			return errors.Wrap(err, "read chunk stream")
		}
		logger.WithField("data", fmt.Sprintf("%#v", cs)).Trace("")

		switch cs.MsgTypeID {
		case MsgAMF0CommandMessage, MsgAMF3CommandMessage:
			if err := c.decodeCommandMessage(cs); err != nil {
				logger.WithField("action", "decodeCommandMessage").Error(err)
				return errors.Wrap(err, "decode command message")
			}
		}

		if c.handleCommandMessageDone {
			break
		}
	}

	return nil
}

func (c *Conn) discoverTcUrl() error {
	u, err := url.Parse(c.tcUrl)
	if err != nil {
		return err
	}
	c.logger.Tracef("tcUrl: %#v", u)

	if strings.ToLower(u.Scheme) != "rtmp" {
		return errors.Errorf("not rtmp scheme: %s", u.Scheme)
	}

	c.rawQuery = u.RawQuery // vhost=...&token=...
	c.urlValues, _ = url.ParseQuery(c.rawQuery)

	parseVhost := func() {
		if v, ok := c.urlValues["vhost"]; ok {
			c.vhost = v[0]
		}
	}

	if strings.Contains(u.Host, ":") { //host:port
		host, port, err := net.SplitHostPort(u.Host)
		if err != nil {
			return errors.Wrap(err, "split host port")
		}
		c.host = host
		c.port, _ = strconv.Atoi(port)

		if u.Host == c.LocalAddr().String() { // ip:port
			//need to parse vhost parameter
			parseVhost()
		} else { // domain:port
			c.vhost = host
		}
	} else { // host
		c.host = u.Host

		lIp, lPort, _ := net.SplitHostPort(c.LocalAddr().String())
		if c.host == lIp {
			// need to parse vhost parameter
			parseVhost()
		} else {
			c.vhost = c.host
		}
		c.port, _ = strconv.Atoi(lPort)
	}

	if c.vhost == "" {
		c.vhost = "_defaultVhost_"
	}

	if idx := strings.Index(c.appName, "?"); idx > 0 {
		c.appName = c.appName[:idx]
		c.appName = strings.Trim(c.appName, " ")
	}

	return nil
}

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
			c.logger.WithField("event", "decode Publish Msg").Trace("success")
		case cmdPlay:
			if err := c.decodePlayCmdMessage(vs[1:]); err != nil {
				return err
			}
			if err := c.respPlayCmdMessage(cs); err != nil {
				return err
			}

			c.handleCommandMessageDone = true
			c.isPublisher = false
			c.logger.WithField("event", "decode Play Msg").Trace("success")
		case cmdFCUnpublish, cmdDeleteStream:
		default:
			//err := fmt.Errorf("unsupport command=%s", cmdStr)
			c.logger.WithField("event", "parse AMF command").Infof(fmt.Sprintf("unsupport command '%s'", cmdStr))
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
		"data": fmt.Sprintf("transactionID: %d, app: '%s', flashVer: '%s', swfUrl: '%s', tcUrl: '%s', objectEncoding: %d",
			c.transactionID, c.appName, c.flashVer, c.swfUrl, c.tcUrl, c.objectEncoding),
	}).Trace("")

	return nil
}

func (c *Conn) respConnectCmdMessage(cs *ChunkStream) error {
	// WindowAcknowledgement Size
	respCs := NewProtolControlMessage(MsgWindowAcknowledgementSize, 4, c.localWindowAckSize)
	if err := c.writeChunkStream(respCs); err != nil {
		c.logger.WithField("event", "Set WindowAckSize Message").Error(err)
		return err
	}
	c.logger.WithField("event", "Set WindowAckSize Message").Trace("success")

	// Set Peer Bandwidth
	respCs = NewProtolControlMessage(MsgSetPeerBandwidth, 5, 2500000)
	respCs.ChunkBody[4] = 2
	if err := c.writeChunkStream(respCs); err != nil {
		c.logger.WithField("event", "Set Peer Bandwidth").Error(err)
		return err
	}
	c.logger.WithField("event", "Set Peer Bandwidth").Trace("success")

	// set chunk size
	respCs = NewProtolControlMessage(MsgSetChunkSize, 4, c.localChunksize)
	if err := c.writeChunkStream(respCs); err != nil {
		c.logger.WithField("event", "Set Chunk Size").Error(err)
		return err
	}
	c.logger.WithField("event", "Set Chunk Size").Trace("success")

	// NetConnection.Connect.Success
	resp := make(amf.Object)
	resp["fmsVer"] = "FMS/3,0,1,123"
	resp["capabilities"] = 31

	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetConnection.Connect.Success"
	event["description"] = "Connection succeeded."
	event["objectEncoding"] = c.objectEncoding
	if err := c.writeCommandMessage(cs.Csid, cs.MsgStreamID, "_result", c.transactionID, resp, event); err != nil {
		c.logger.WithField("event", "NetConnection.Connect.Success").Error(err)
		return err
	}
	c.logger.WithField("event", "NetConnection.Connect.Success").Trace("success")

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
	return c.writeCommandMessage(cs.Csid, cs.MsgStreamID, "_result", c.transactionID, nil, 1 /*streamID*/)
}

func (c *Conn) decodePulishCmdMessage(vs []interface{}) error {
	return c.publishOrPlay(vs)
}

func (c *Conn) respPulishCmdMessage(cs *ChunkStream) error {
	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetStream.Publish.Start"
	event["description"] = "Start publising."

	return c.writeCommandMessage(cs.Csid, cs.MsgStreamID, "onStatus", 0, nil, event)
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
	if err := c.writeChunkStream(cs1); err != nil {
		return errors.Wrap(err, "send user control message streamIsRecorded")
	}

	// set begin
	cs2 := NewUserControlMessage(streamBegin, 4)
	for i := 0; i < 4; i++ {
		cs2.ChunkBody[i+2] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	if err := c.writeChunkStream(cs2); err != nil {
		return errors.Wrap(err, "send user control message streamBegin")
	}

	// NetStream.Play.Resetstream
	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetStream.Play.Reset"
	event["description"] = "Playing and resetting stream."
	if err := c.writeCommandMessage(cs.Csid, cs.MsgStreamID, "onStatus", 0, nil, event); err != nil {
		return errors.Wrap(err, "send NetStream.Play.Reset message")
	}

	// NetStream.Play.Start
	event["level"] = "status"
	event["code"] = "NetStream.Play.Start"
	event["description"] = "Started playing stream."
	if err := c.writeCommandMessage(cs.Csid, cs.MsgStreamID, "onStatus", 0, nil, event); err != nil {
		return errors.Wrap(err, "send NetStream.Play.Start message")
	}

	// NetStream.Data.Start
	event["level"] = "status"
	event["code"] = "NetStream.Data.Start"
	event["description"] = "Started playing stream."
	if err := c.writeCommandMessage(cs.Csid, cs.MsgStreamID, "onStatus", 0, nil, event); err != nil {
		return errors.Wrap(err, "send NetStream.Data.Start message")
	}

	// NetStream.Play.PublishNotify
	event["level"] = "status"
	event["code"] = "NetStream.Play.PublishNotify"
	event["description"] = "Started playing notify."
	if err := c.writeCommandMessage(cs.Csid, cs.MsgStreamID, "onStatus", 0, nil, event); err != nil {
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
func (c *Conn) writeCommandMessage(csid, streamID uint32, args ...interface{}) error {
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
		msgHdrBuf: make([]byte, 11),
	}

	if err := c.writeChunkStream(&cs); err != nil {
		return err
	}

	return nil
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
