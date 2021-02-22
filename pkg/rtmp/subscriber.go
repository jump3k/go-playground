package rtmp

import (
	"encoding/binary"
	"errors"
	"playground/pkg/av"

	"github.com/gwuhaolin/livego/protocol/amf"
	"github.com/sirupsen/logrus"
)

type subscriber struct {
	rtmpConn *Conn

	stopped bool
	subType string // "gerneral"
	logger  *logrus.Logger

	avPktQueue     chan *av.Packet
	avPktQueueSize int //av packet buffer size

	initCache          bool
	baseTimeStamp      uint32
	lastAudioTimeStamp uint32
	lastVideoTimeStamp uint32
	chunkMsgToSend     *ChunkStream
}

func newSubscriber(c *Conn, avQueueSize int) *subscriber {
	sub := &subscriber{
		rtmpConn:       c,
		subType:        "gerneral",
		logger:         c.logger,
		avPktQueue:     make(chan *av.Packet, avQueueSize),
		avPktQueueSize: avQueueSize,
		chunkMsgToSend: new(ChunkStream),
	}

	return sub
}

func (s *subscriber) sendCachePacket(cache *Cache) {
	if s.initCache {
		return
	}

	metaData := cache.metaData
	if metaData.full && metaData.pkt != nil {
		s.writeAVPacket(metaData.pkt)
	}

	videoSeq := cache.videoSeq
	if videoSeq.full && videoSeq.pkt != nil {
		s.writeAVPacket(videoSeq.pkt)
	}

	audioSeq := cache.audioSeq
	if audioSeq.full && audioSeq.pkt != nil {
		s.writeAVPacket(audioSeq.pkt)
	}

	s.initCache = true
}

func (s *subscriber) playingCycle(ss *streamSource) error {
	for {
		pkt, ok := <-s.avPktQueue
		if !ok {
			s.stopped = true
			return errors.New("closed")
		}

		if err := s.sendAVPacket(pkt); err != nil {
			s.stopped = true
			return err
		}
		s.logger.WithField("event", "SendAVPacket").Debugf("pkt: %+v", pkt)
	}
}

func (s *subscriber) sendAVPacket(pkt *av.Packet) error {
	cs := s.chunkMsgToSend

	cs.ChunkBody = pkt.Data
	cs.MsgLength = uint32(len(pkt.Data))
	cs.MsgStreamID = pkt.StreamID
	cs.TimeStamp = pkt.TimeStamp + s.getBaseTimeStamp()

	switch {
	case pkt.IsVideo:
		cs.MsgTypeID = MsgVideoMessage
	case pkt.IsAudio:
		cs.MsgTypeID = MsgAudioMessage
	case pkt.IsMetaData:
		cs.MsgTypeID = MSGAMF0DataMessage
	}

	s.recordTimeStamp(cs.MsgTypeID, cs.TimeStamp)

	return s.writeAVChunkStream(cs)
}

func (s *subscriber) writeAVChunkStream(cs *ChunkStream) error {
	switch cs.MsgTypeID {
	case MsgAMF3DataMessage, MSGAMF0DataMessage:
		var err error
		if cs.ChunkBody, err = amf.MetaDataReform(cs.ChunkBody, amf.DEL); err != nil {
			return err
		}
		cs.MsgLength = uint32(len(cs.ChunkBody))
	case MsgSetChunkSize:
		s.rtmpConn.localChunksize = binary.BigEndian.Uint32(cs.ChunkBody)
	}

	return s.rtmpConn.writeChunkStream(cs)
}

func (s *subscriber) writeAVPacket(pkt *av.Packet) {
	//s.logger.WithField("event", "avpkt enQueue").Infof("data len: %d", len(pkt.Data))
	if len(s.avPktQueue) > s.avPktQueueSize-24 {
		s.dropAVPacket()
	} else {
		s.avPktQueue <- pkt
	}
}

func (s *subscriber) dropAVPacket() {
	//s.logger.WithField("event", "dropAvPkt").Infof("subscriber: %s", s.rtmpConn.RemoteAddr().String())
	for i := 0; i < s.avPktQueueSize-84; i++ {
		pkt, ok := <-s.avPktQueue
		if !ok {
			continue
		}

		switch {
		case pkt.IsAudio:
			if len(s.avPktQueue) > s.avPktQueueSize-2 {
				s.logger.WithField("event", "dropAvPkt").Infof("drop audio pkt")
				<-s.avPktQueue
			} else {
				s.avPktQueue <- pkt //enqueu again
			}
		case pkt.IsVideo:
			vPkt, ok := pkt.Header.(av.VideoPacketHeader)
			if ok && (vPkt.IsSeq() || vPkt.IsKeyFrame()) {
				s.avPktQueue <- pkt
			}

			if len(s.avPktQueue) > s.avPktQueueSize-10 {
				s.logger.WithField("event", "dropAvPkt").Infof("drop audio pkt")
				<-s.avPktQueue
			}
		}
	}
}

func (s *subscriber) recordTimeStamp(msgTypeID RtmpMsgTypeID, timeStamp uint32) {
	switch msgTypeID {
	case MsgVideoMessage:
		s.lastVideoTimeStamp = timeStamp
	case MsgAudioMessage:
		s.lastAudioTimeStamp = timeStamp
	}

	// calcBaseTimeStamp
	if s.lastAudioTimeStamp > s.lastVideoTimeStamp {
		s.baseTimeStamp = s.lastAudioTimeStamp
	} else {
		s.baseTimeStamp = s.lastVideoTimeStamp
	}
}

func (s *subscriber) getBaseTimeStamp() uint32 {
	return s.baseTimeStamp
}
