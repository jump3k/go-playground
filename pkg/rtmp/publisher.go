package rtmp

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"playground/pkg/av"
	"playground/pkg/flv"
)

type publisher struct {
	rtmpConn  *Conn
	streamKey string

	demuxer *flv.Demuxer
	logger  *logrus.Logger
}

func newPublisher(c *Conn, streamKey string) *publisher {
	p := &publisher{
		rtmpConn:  c,
		streamKey: streamKey,
		demuxer:   flv.NewDemuxer(),
		logger:    c.logger,
	}

	return p
}

func (p *publisher) publishingCycle(ss *streamSource) error {
	// start to recv av data
	basicHdrBuf := make([]byte, 3) // rtmp chunk basic header, at most 3 bytes

loopRecvAVChunkStream:
	for {
		cs, err := p.rtmpConn.readChunkStream(basicHdrBuf)
		if err != nil {
			p.logger.WithField("event", "recv av chunk stream").Error(err)
			return err
		}
		p.logger.WithField("event", "recv av chunk stream").Tracef("data: %s", fmt.Sprintf("%#v", cs))

		avPkt := new(av.Packet)
		switch cs.MsgTypeID {
		case MsgAudioMessage:
			avPkt.IsAudio = true
		case MsgVideoMessage:
			avPkt.IsVideo = true
		case MSGAMF0DataMessage, MsgAMF3DataMessage:
			avPkt.IsMetaData = true
		default:
			continue loopRecvAVChunkStream
		}

		avPkt.StreamID = cs.MsgStreamID
		avPkt.Data = cs.ChunkBody

		ss.cache.Write(avPkt)
		ss.dispatchAvPkt(cs, avPkt) //dispatch av pkt
	}
}

/*
func (p *publisher) close() {
	//p.pubMgr.deletePublisher(p.streamKey)
	val, ok := p.ssMgr.streamMap.Load(p.streamKey)
	if ok {
		ss := val.(*streamSource)
		ss.publisher = nil
	}

	time.AfterFunc(time.Minute, func() { // check after 1min
		val, ok := p.ssMgr.streamMap.Load(p.streamKey)
		if ok {
			ss := val.(*streamSource)
			if ss.publisher == nil {
				p.ssMgr.streamMap.Delete(p.streamKey) //delete actual
				_ = p.logger.Log("level", "INFO", "event", fmt.Sprintf("delete %s from streamMgr", p.streamKey))
			}
		}
	})
}
*/
