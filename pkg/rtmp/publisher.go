package rtmp

import (
	"github.com/sirupsen/logrus"

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

func (p *publisher) publishingCycle() error {
	// start to recv av data
	basicHdrBuf := make([]byte, 3) // rtmp chunk basic header, at most 3 bytes
	for {
		cs, err := p.rtmpConn.readChunkStream(basicHdrBuf)
		if err != nil {
			p.logger.WithField("event", "recv av stream").Error(err)
			return err
		}
		//_ = p.logger.Log("level", "ERROR", "event", "recv av chunk stream", "data", fmt.Sprintf("%#v", cs))

		switch cs.MsgTypeID {
		case MsgAudioMessage, MsgVideoMessage, MSGAMF0DataMessage, MsgAMF3DataMessage: //audio/video relational data
			//TODO:demux av data
			avPkt := cs.decodeAVChunkStream()
			//_ = p.logger.Log("level", "INFO", "event", "AVPKT", "a:", avPkt.IsAudio, "v:", avPkt.IsVideo, "meta:", avPkt.IsMetaData)

			err := p.demuxer.DemuxHdr(avPkt)
			if err != nil {
				return err
			}

			/*
				val, ok := p.rtmpConn.ssMgr.streamMap.Load(p.streamKey)
				if !ok {
					_ = p.logger.Log("level", "FATAL", "event", "publishingCycle", "error", "streamSource not found while publishing")
					break
				}

				subs := val.(*streamSource).subs
				for _, sub := range subs {
					_ = sub
					//TODO: send data to subscriber
				}
			*/
		default:
		}
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
