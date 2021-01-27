package rtmp

import (
	"encoding/binary"
	"fmt"
	"io"
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
	ChunkData []byte

	//TODO: help variable here?
	tmpFormat    uint8
	timeExtended bool
	gotFull      bool
	index        uint32
	remain       uint32
}

func NewChunkStream() *ChunkStream {
	return &ChunkStream{}
}

func (cs *ChunkStream) SetControlMessage(typeID RtmpMsgTypeID, length uint32, value uint32) *ChunkStream {
	cs.Fmt = 0
	cs.Csid = 2
	cs.MsgTypeID = typeID
	cs.MsgStreamID = 0
	cs.MsgLength = length
	cs.ChunkData = make([]byte, length)
	putU32BE(cs.ChunkData[:length], value)

	return cs
}

func (c *Conn) ReadChunkStream() (*ChunkStream, error) {
	for {
		h, err := c.ReadUint(1, true)
		if err != nil {
			return nil, err
		}

		fmt := h >> 6
		csid := h & 0x3f
		cs, ok := c.chunks[csid]
		if !ok {
			cs = &ChunkStream{}
			c.chunks[csid] = cs
		}

		cs.tmpFormat = uint8(fmt)
		cs.ChunkBasicHeader.Csid = csid
		if err := c.readChunk(cs); err != nil {
			return nil, err
		}
		c.chunks[csid] = cs

		if cs.gotFull {
			return cs, nil
		}
	}
}

func (c *Conn) readChunk(cs *ChunkStream) error {
	if cs.remain != 0 && cs.tmpFormat != 3 {
		return fmt.Errorf("remain(%d) not zero while tmpFormat as 1/2/3", cs.remain)
	}

	switch cs.Csid {
	case 0:
		id, _ := c.ReadUint(1, false)
		cs.Csid = id + 64
	case 1:
		id, _ := c.ReadUint(2, false)
		cs.Csid = id + 64
	}

	setRemainFlag := func(cs *ChunkStream) {
		cs.gotFull = false
		cs.index = 0
		cs.remain = cs.MsgLength
		cs.ChunkData = make([]byte, int(cs.MsgLength))
	}

	switch cs.tmpFormat {
	case 0:
		cs.Fmt = 0
		cs.TimeStamp, _ = c.ReadUint(3, true)
		cs.MsgLength, _ = c.ReadUint(3, true)
		msgTypeId, _ := c.ReadUint(1, true)
		cs.MsgTypeID = RtmpMsgTypeID(msgTypeId)
		cs.MsgStreamID, _ = c.ReadUint(4, false)

		cs.timeExtended = false
		if cs.TimeStamp == 0xffffff {
			cs.TimeStamp, _ = c.ReadUint(4, true)
			cs.timeExtended = true
		}
		setRemainFlag(cs)
	case 1:
		cs.Fmt = 1
		timeStamp, _ := c.ReadUint(3, true)
		cs.MsgLength, _ = c.ReadUint(3, true)
		msgTypeId, _ := c.ReadUint(1, true)
		cs.MsgTypeID = RtmpMsgTypeID(msgTypeId)

		cs.timeExtended = false
		if timeStamp == 0xffffff {
			timeStamp, _ = c.ReadUint(4, true)
			cs.timeExtended = true
		}

		cs.ExtendedTimeStamp = timeStamp
		cs.TimeStamp += timeStamp
		setRemainFlag(cs)
	case 2:
		cs.Fmt = 2
		timeStamp, _ := c.ReadUint(3, true)

		cs.timeExtended = false
		if timeStamp == 0xffffff {
			timeStamp, _ = c.ReadUint(4, true)
			cs.timeExtended = true
		}

		cs.ExtendedTimeStamp = timeStamp
		cs.TimeStamp += timeStamp
		setRemainFlag(cs)
	case 3:
		if cs.remain == 0 {
			switch cs.Fmt {
			case 0:
				if cs.timeExtended {
					cs.TimeStamp, _ = c.ReadUint(4, true)
				}
			case 1, 2:
				timedelta := cs.ExtendedTimeStamp
				if cs.timeExtended {
					timedelta, _ = c.ReadUint(4, true)
				}
				cs.TimeStamp += timedelta
			}
			setRemainFlag(cs)
		} else {
			if cs.timeExtended {
				b := make([]byte, 4)
				if _, err := io.ReadAtLeast(c, b, 4); err != nil { //TODO: peek
					return err
				}

				tmpTimeStamp := binary.BigEndian.Uint32(b)
				if tmpTimeStamp == cs.TimeStamp {
					// discard 4 bytes
				} else {
					//TODO: if peek, delete the next three lines
					copy(cs.ChunkData[cs.index:cs.index+4], b)
					cs.index += 4
					cs.remain -= 4
				}
			}
		}
	default:
		return fmt.Errorf("invalid rtmp format: %d", cs.Fmt)
	}

	size := cs.remain
	if size > c.localChunksize {
		size = c.localChunksize
	}

	buf := cs.ChunkData[cs.index : cs.index+size]
	if n, err := c.Read(buf); err != nil {
		return err
	} else {
		cs.index += uint32(n)
		cs.remain -= uint32(n)

		if cs.remain == 0 {
			cs.gotFull = true
		}
	}

	return nil
}

func (c *Conn) ReadUint(n int, bigEndian bool) (uint32, error) {
	ret := uint32(0)

	oneByte := make([]byte, 1)
	for i := 0; i < n; i++ {
		_, err := io.ReadAtLeast(c, oneByte, 1)
		if err != nil {
			return 0, err
		}

		if bigEndian { // big endian
			ret = ret<<8 + uint32(oneByte[0])
		} else { // little endian
			ret += uint32(oneByte[0]) << uint32(i*8)
		}
	}

	return ret, nil
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

const (
	cmdConnect       = "connect"
	cmdFcpublish     = "FCPublish"
	cmdReleaseStream = "releaseStream"
	cmdCreateStream  = "createStream"
	cmdPublish       = "publish"
	cmdFCUnpublish   = "FCUnpublish"
	cmdDeleteStream  = "deleteStream"
	cmdPlay          = "play"
)
