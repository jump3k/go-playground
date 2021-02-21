package rtmp

import (
	"encoding/binary"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type ChunkBasicHeader struct {
	Fmt  uint8
	Csid uint32
}

type ChunkMessageHeader struct {
	TimeStamp   uint32
	MsgLength   uint32
	MsgTypeID   RtmpMsgTypeID
	MsgStreamID uint32
}

type ChunkHeader struct {
	ChunkBasicHeader
	ChunkMessageHeader
	ExtendedTimeStamp uint32
}

type ChunkStream struct {
	ChunkHeader
	ChunkBody []byte

	msgHdrSize int
	msgHdrBuf  []byte // At most 11bytes

	timeExtended bool
	gotBodyFull  bool
	bodyIndex    uint32
	bodyRemain   uint32
}

func newChunkStream() *ChunkStream {
	return &ChunkStream{}
}

func NewProtolControlMessage(typeID RtmpMsgTypeID, length uint32, value uint32) *ChunkStream {
	cs := &ChunkStream{
		ChunkHeader: ChunkHeader{
			ChunkBasicHeader: ChunkBasicHeader{
				Fmt:  0,
				Csid: 2,
			},
			ChunkMessageHeader: ChunkMessageHeader{
				MsgTypeID:   typeID,
				MsgStreamID: 0,
				MsgLength:   length,
			},
		},
		ChunkBody: make([]byte, length),
	}

	//putU32BE(cs.ChunkBody[:length], value)
	uintAsbyteSlice(value, cs.ChunkBody[:4], true)

	return cs
}

func NewUserControlMessage(eventType, buflen uint32) *ChunkStream {
	buflen += 2

	cs := &ChunkStream{
		ChunkHeader: ChunkHeader{
			ChunkBasicHeader: ChunkBasicHeader{
				Fmt:  0,
				Csid: 2,
			},
			ChunkMessageHeader: ChunkMessageHeader{
				MsgTypeID:   MsgUserControlMessage,
				MsgStreamID: 1,
				MsgLength:   buflen,
			},
		},
		ChunkBody: make([]byte, buflen),
	}

	cs.ChunkBody[0] = byte(eventType >> 8 & 0xff)
	cs.ChunkBody[1] = byte(eventType & 0xff)

	return cs
}

/*
func (cs *ChunkStream) decodeAVChunkStream(pkt *av.Packet) *av.Packet {
	switch cs.MsgTypeID {
	case MsgAudioMessage:
		pkt.IsAudio = true
	case MsgVideoMessage:
		pkt.IsVideo = true
	case MSGAMF0DataMessage, MsgAMF3DataMessage:
		pkt.IsMetaData = true
	}

	pkt.StreamID = cs.MsgStreamID
	pkt.Data = cs.ChunkBody

	return pkt
}
*/

//read one chunk stream fully
func (c *Conn) readChunkStream(basicHdrBuf []byte) (*ChunkStream, error) {
	for {
		fmt, csid, err := c.readChunkBasicHeader(basicHdrBuf)
		if err != nil {
			return nil, err
		}

		cs, ok := c.chunks[csid]
		if !ok {
			cs = newChunkStream()
			cs.Fmt = fmt
			cs.Csid = csid
			cs.msgHdrBuf = make([]byte, 11)
			c.chunks[cs.Csid] = cs
		}

		if err := c.readChunkMessageHeader(cs, fmt); err != nil {
			return nil, err
		}

		if err := c.readChunkMessageBody(cs); err != nil {
			return nil, err
		}

		if cs.gotBodyFull {
			c.onReadChunkStreamSucc(cs)
			return cs, nil
		}
	}
}

func (c *Conn) readChunkBasicHeader(basicHdrBuf []byte) (uint8, uint32, error) {
	h, err := c.ReadUint(basicHdrBuf[0:1], true)
	if err != nil {
		return 0, 0, errors.Wrap(err, "basic header requires 1 bytes")
	}

	fmt := uint8(h >> 6)
	csid := h & 0x3f

	switch csid {
	case 0: // 64-319, 2Bytes chunk basic header
		id, err := c.ReadUint(basicHdrBuf[1:2], false)
		if err != nil {
			return fmt, csid, errors.Wrap(err, "basic header requires 2 bytes")
		}
		csid = id + 64
	case 1: // 64-65599, 3Bytes chunk basic header
		id, err := c.ReadUint(basicHdrBuf[1:3], false)
		if err != nil {
			return fmt, csid, errors.Wrap(err, "basic header requires 3 bytes")
		}
		csid = id + 64
	default: // 2-63, 1Byte chunk basic header
		// csid > 1
	}

	return fmt, csid, nil
}

func (c *Conn) readChunkMessageHeader(cs *ChunkStream, fmt uint8) error {
	switch fmt {
	case 0:
		cs.msgHdrSize = 11
	case 1:
		cs.msgHdrSize = 7
	case 2:
		cs.msgHdrSize = 3
	case 3:
		cs.msgHdrSize = 0
	}

	var buf []byte
	if cs.msgHdrSize > 0 {
		buf = cs.msgHdrBuf[0:cs.msgHdrSize]
		if nr, err := c.readWriter.Read(buf); err != nil || nr != cs.msgHdrSize {
			return errors.Wrapf(err, "read %d bytes message header", cs.msgHdrSize)
		}
	}

	/*
	 * parse the message header.
	 *   3bytes: timestamp delta,    fmt=0,1,2
	 *   3bytes: payload length,     fmt=0,1
	 *   1bytes: message type,       fmt=0,1
	 *   4bytes: stream id,          fmt=0
	 */
	if fmt <= 2 {
		cs.ExtendedTimeStamp = byteSliceAsUint(buf[0:3], true) // timestamp (delta)
		cs.timeExtended = cs.ExtendedTimeStamp >= 0xffffff

		if !cs.timeExtended {
			switch cs.Fmt {
			case 0:
				cs.TimeStamp = cs.ExtendedTimeStamp
			case 1, 2:
				cs.TimeStamp += cs.ExtendedTimeStamp
			}
		}

		if fmt <= 1 {
			payloadLength := byteSliceAsUint(buf[3:6], true) // payload length
			cs.MsgLength = payloadLength

			msgTypeID := byteSliceAsUint(buf[6:7], true) // message type
			cs.MsgTypeID = RtmpMsgTypeID(msgTypeID)

			if fmt == 0 {
				msgStreamID := byteSliceAsUint(buf[7:11], false) // stream id
				cs.MsgStreamID = msgStreamID
			}
		}

		cs.gotBodyFull = false
		cs.bodyIndex = 0
		cs.bodyRemain = cs.MsgLength
		cs.ChunkBody = make([]byte, int(cs.MsgLength))
	} else {
		if cs.bodyRemain == 0 {
			switch cs.Fmt {
			case 0:
				if cs.timeExtended {
					b := make([]byte, 4)
					cs.TimeStamp, _ = c.ReadUint(b, true)
				}
			case 1, 2:
				timedelta := cs.ExtendedTimeStamp
				if cs.timeExtended {
					b := make([]byte, 4)
					timedelta, _ = c.ReadUint(b, true)
				}
				cs.TimeStamp += timedelta
			}

			cs.gotBodyFull = false
			cs.bodyIndex = 0
			cs.bodyRemain = cs.MsgLength
			cs.ChunkBody = make([]byte, int(cs.MsgLength))
		} else {
			if cs.timeExtended {
				b, err := c.readWriter.Peek(4)
				if err != nil {
					return err
				}

				tmpTimeStamp := binary.BigEndian.Uint32(b)
				if tmpTimeStamp == cs.TimeStamp {
					_, _ = c.readWriter.Discard(4)
				}
			}
		}
	}

	return nil
}

func (c *Conn) readChunkMessageBody(cs *ChunkStream) error {
	size := cs.bodyRemain
	if size > c.remoteChunkSize {
		size = c.remoteChunkSize //important: read chunk from peer accord to min(remoteChunkSize, cs.remain)
	}

	buf := cs.ChunkBody[cs.bodyIndex : cs.bodyIndex+size]
	if nr, err := c.readWriter.Read(buf); err != nil || nr != int(size) {
		return errors.Wrapf(err, "read %d bytes, autual: %d", size, nr)
	} else {
		cs.bodyIndex += uint32(nr)
		cs.bodyRemain -= uint32(nr)

		if cs.bodyRemain == 0 {
			cs.gotBodyFull = true
		}
	}

	return nil
}

// write one chunk stream fully
func (c *Conn) writeChunkStream(cs *ChunkStream) error {
	switch cs.MsgTypeID {
	case MsgAudioMessage:
		cs.Csid = 4
	case MsgVideoMessage, MsgAMF3DataMessage, MSGAMF0DataMessage:
		cs.Csid = 6
	}

	totalLen := uint32(0)
	numChunks := (cs.MsgLength / c.localChunksize) // split by local chunk size
	for i := uint32(0); i <= numChunks; i++ {
		if totalLen == cs.MsgLength {
			break
		}

		if i == 0 {
			cs.Fmt = 0
		} else {
			cs.Fmt = 3
		}

		if err := c.writeChunkHeader(cs); err != nil { //write rtmp chunk header
			return err
		}

		inc := c.localChunksize
		start := i * c.localChunksize

		leftLen := uint32(len(cs.ChunkBody)) - start
		if leftLen < c.localChunksize {
			inc = leftLen
		}
		totalLen += inc

		buf := cs.ChunkBody[start : start+inc]
		if _, err := c.readWriter.Write(buf); err != nil { //write rtmp chunk body
			return err
		}
	}

	if err := c.readWriter.Flush(); err != nil {
		return err
	}

	return nil
}

func (c *Conn) onReadChunkStreamSucc(cs *ChunkStream) {
	switch cs.MsgTypeID {
	case MsgSetChunkSize:
		c.remoteChunkSize = binary.BigEndian.Uint32(cs.ChunkBody)
		c.logger.WithFields(logrus.Fields{"event": "save remoteChunkSize", "data": c.remoteChunkSize}).Trace("")
	case MsgWindowAcknowledgementSize:
		c.remoteWindowAckSize = binary.BigEndian.Uint32(cs.ChunkBody)
		c.logger.WithFields(logrus.Fields{"event": "save remoteWindowAckSize", "data": c.remoteWindowAckSize}).Trace("")
	default:
	}

	c.ack(cs.MsgLength)
}

func (c *Conn) ack(size uint32) {
	c.bytesRecv += size
	if c.bytesRecv >= 1<<32-1 {
		c.bytesRecv = 0
		c.bytesRecvReset++
	}

	c.ackSeqNumber += size
	if c.ackSeqNumber >= c.remoteWindowAckSize { //超过窗口通告大小，回复ACK
		cs := NewProtolControlMessage(MsgAcknowledgement, 4, c.ackSeqNumber)
		if err := c.writeChunkStream(cs); err != nil {
			c.logger.WithFields(logrus.Fields{"event": "send ACK"}).Error(err)
		}

		c.ackSeqNumber = 0
	}
}

func (c *Conn) writeChunkHeader(cs *ChunkStream) error {
	// write basic header
	h := uint32(cs.Fmt) << 6
	switch {
	case cs.Csid < 64:
		h |= cs.Csid
		if err := c.writeUint(h, c.basicHdrBuf[0:1], false); err != nil {
			return err
		}
	case cs.Csid-64 < 256:
		h |= 0
		if err := c.writeUint(h, c.basicHdrBuf[0:1], false); err != nil {
			return err
		}

		if err := c.writeUint(cs.Csid-64, c.basicHdrBuf[0:1], false); err != nil {
			return err
		}
	case cs.Csid-64 < 65536:
		h |= 1
		if err := c.writeUint(h, c.basicHdrBuf[0:1], false); err != nil {
			return err
		}

		if err := c.writeUint(cs.Csid-64, c.basicHdrBuf[0:2], false); err != nil {
			return err
		}
	}

	// write message header
	if cs.msgHdrBuf == nil {
		cs.msgHdrBuf = make([]byte, 11)
	}
	ts := cs.TimeStamp
	if cs.Fmt == 3 {
		goto END
	}

	if cs.TimeStamp > 0xffffff {
		ts = 0xffffff
	}
	if err := c.writeUint(ts, cs.msgHdrBuf[0:3], true); err != nil {
		return err
	}

	if cs.Fmt == 2 {
		goto END
	}

	if cs.MsgLength > 0xffffff {
		return fmt.Errorf("length=%d", cs.MsgLength)
	}
	if err := c.writeUint(cs.MsgLength, cs.msgHdrBuf[0:3], true); err != nil {
		return err
	}
	if err := c.writeUint(uint32(cs.MsgTypeID), cs.msgHdrBuf[0:1], true); err != nil {
		return err
	}

	if cs.Fmt == 1 {
		goto END
	}
	if err := c.writeUint(cs.MsgStreamID, cs.msgHdrBuf[0:4], false); err != nil {
		return err
	}

END:
	if ts > 0xffffff {
		if err := c.writeUint(cs.TimeStamp, cs.msgHdrBuf[0:4], true); err != nil {
			return err
		}
	}

	return nil
}

func (c *Conn) ReadUint(b []byte, bigEndian bool) (uint32, error) {
	if nr, err := c.readWriter.Read(b); err != nil {
		c.logger.WithFields(logrus.Fields{"event": fmt.Sprintf("read %d byte, actual: %d", len(b), nr)}).Error(err)
		return 0, err
	}

	return byteSliceAsUint(b, bigEndian), nil
}

/*
func (c *Conn) writeUint(val uint32, nbytes int, bigEndian bool) error {
	buf := make([]byte, nbytes)
	uintAsbyteSlice(val, buf, bigEndian)

	if nw, err := c.readWriter.Write(buf); err != nil {
		c.logger.WithFields(logrus.Fields{"event": fmt.Sprintf("write %d byte, actual: %d", nbytes, nw)}).Error(err)
		return err
	}

	return nil
}
*/

func (c *Conn) writeUint(val uint32, buf []byte, bigEndian bool) error {
	uintAsbyteSlice(val, buf, bigEndian)
	if nw, err := c.readWriter.Write(buf); err != nil {
		c.logger.WithFields(logrus.Fields{"event": fmt.Sprintf("write %d byte, actual: %d", len(buf), nw)}).Error(err)
		return err
	}

	return nil
}

func uintAsbyteSlice(val uint32, b []byte, bigEndian bool) {
	nbytes := len(b)
	for i := 0; i < nbytes; i++ {
		if bigEndian {
			v := val >> ((nbytes - i - 1) << 3)
			b[i] = byte(v) & 0xff
		} else {
			b[i] = byte(val) & 0xff
			val = val >> 8
		}
	}
}

func byteSliceAsUint(b []byte, bigEndian bool) uint32 {
	ret := uint32(0)

	n := len(b)
	for i := 0; i < n; i++ {
		if bigEndian { // big endian
			ret = ret<<8 + uint32(b[i])
		} else { // little endian
			ret += uint32(b[i]) << uint32(i*8)
		}
	}

	return ret
}

type RtmpMsgTypeID uint32

const (
	_                             RtmpMsgTypeID = iota
	MsgSetChunkSize                                        //0x01
	MsgAbortMessage                                        //0x02
	MsgAcknowledgement                                     //0x03
	MsgUserControlMessage                                  //0x04
	MsgWindowAcknowledgementSize                           //0x05
	MsgSetPeerBandwidth                                    //0x06
	MsgEdgeAndOriginServerCommand                          //0x07(internal, protocol not define)
	MsgAudioMessage                                        //0x08
	MsgVideoMessage                                        //0x09
	MsgAMF3DataMessage            RtmpMsgTypeID = 5 + iota //0x0F
	MsgAMF3SharedObject                                    //0x10
	MsgAMF3CommandMessage                                  //0x11
	MSGAMF0DataMessage                                     //0x12
	MSGAMF0SharedObject                                    //0x13
	MsgAMF0CommandMessage                                  //0x14
	MsgAggregateMessage           RtmpMsgTypeID = 22       //0x16
)
