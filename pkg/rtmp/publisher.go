package rtmp

import (
	"fmt"

	"github.com/go-kit/kit/log"

	"playground/pkg/flv"
)

type publisher struct {
	rtmpConn  *Conn
	SessionID string
	streamKey string

	demuxer *flv.Demuxer

	pubMgr *publisherMgr
	logger log.Logger
}

func newPublisher(c *Conn, streamKey string) *publisher {
	p := &publisher{
		rtmpConn:  c,
		streamKey: streamKey,
		demuxer:   flv.NewDemuxer(),
		pubMgr:    c.pubMgr,
		logger:    c.logger,
	}

	p.SessionID = genUuid()
	return p
}

func (p *publisher) publishingCycle() {
	// start to recv av data
	for {
		cs, err := p.rtmpConn.readChunkStream()
		if err != nil {
			_ = p.logger.Log("level", "ERROR", "event", "recv av stream", "error", err.Error())
			break
		}
		//_ = p.logger.Log("level", "ERROR", "event", "recv av chunk stream", "data", fmt.Sprintf("%#v", cs))

		switch cs.MsgTypeID {
		case MsgAudioMessage, MsgVideoMessage, MSGAMF0DataMessage, MsgAMF3DataMessage: //audio/video relational data
			//TODO:demux av data
			avPkt := cs.decodeAVChunkStream()
			//_ = p.logger.Log("level", "INFO", "event", "AVPKT", "a:", avPkt.IsAudio, "v:", avPkt.IsVideo, "meta:", avPkt.IsMetaData)

			err := p.demuxer.DemuxHdr(avPkt)
			if err != nil {
				break
			}
		default:
		}
	}
}

func (p *publisher) close() {
	p.pubMgr.deletePublisher(p.streamKey)
	_ = p.logger.Log("level", "INFO", "event", fmt.Sprintf("delete %s from streamMgr", p.streamKey))
}
