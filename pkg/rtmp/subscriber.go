package rtmp

import (
	"errors"
	"playground/pkg/av"

	"github.com/sirupsen/logrus"
)

type subscriber struct {
	rtmpConn  *Conn
	streamKey string

	stopSub <-chan bool
	subType string // "gerneral"
	logger  *logrus.Logger

	avPktQueue     chan *av.Packet
	avPktQueueSize int //av packet buffer size

	baseTimeStamp      uint32
	lastAudioTimeStamp uint32
	lastVideoTimeStamp uint32
}

func newSubscriber(c *Conn, avQueueSize int) *subscriber {
	sub := &subscriber{
		rtmpConn:       c,
		subType:        "gerneral",
		logger:         c.logger,
		stopSub:        make(<-chan bool, 1),
		avPktQueue:     make(chan *av.Packet, avQueueSize),
		avPktQueueSize: avQueueSize,
	}

	sub.streamKey = genStreamKey(c.domain, c.appName, c.streamName)
	return sub
}

func (s *subscriber) playingCycle(ss *streamSource) error {
	cs := new(ChunkStream)

	for {
		pkt, ok := <-s.avPktQueue
		if !ok {
			return errors.New("closed")
		}

		cs.ChunkBody = pkt.Data
		cs.MsgLength = uint32(len(pkt.Data))
		cs.MsgStreamID = pkt.StreamID
		cs.TimeStamp += s.baseTimeStamp

		switch {
		case pkt.IsVideo:
			cs.MsgTypeID = MsgVideoMessage
		case pkt.IsAudio:
			cs.MsgTypeID = MsgAudioMessage
		case pkt.IsMetaData:
			cs.MsgTypeID = MSGAMF0DataMessage
		}

		if err := s.rtmpConn.writeChunStream(cs); err != nil {
			return err
		}
	}
}

func (s *subscriber) avPktEnQueue(pkt *av.Packet) {
	if len(s.avPktQueue) > s.avPktQueueSize-24 {
		//TODO: DROP
	} else {
		s.avPktQueue <- pkt
	}
}

/*
func (s *subscriber) avPktDeQueue() {
}
*/

func (s *subscriber) recordTimeStamp(cs *ChunkStream) {
	switch cs.MsgTypeID {
	case MsgVideoMessage:
		s.lastVideoTimeStamp = cs.TimeStamp
	case MsgAudioMessage:
		s.lastAudioTimeStamp = cs.TimeStamp
	}

	if s.lastAudioTimeStamp > s.lastVideoTimeStamp {
		s.baseTimeStamp = s.lastAudioTimeStamp //set max
	} else {
		s.baseTimeStamp = s.lastVideoTimeStamp
	}
}
