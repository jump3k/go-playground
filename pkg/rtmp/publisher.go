package rtmp

import (
	"fmt"

	"github.com/go-kit/kit/log"
)

type publisher struct {
	rtmpConn  *Conn
	SessionID string
	streamKey string

	pubMgr *publisherMgr
	logger log.Logger
}

func newPublisher(c *Conn, streamKey string) *publisher {
	p := &publisher{
		rtmpConn:  c,
		streamKey: streamKey,
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
		_ = p.logger.Log("level", "ERROR", "event", "recv av chunk stream", "data", fmt.Sprintf("%#v", cs))
	}
}

func (p *publisher) close() {
	p.pubMgr.deletePublisher(p.streamKey)
	_ = p.logger.Log("level", "INFO", "event", fmt.Sprintf("delete %s from streamMgr", p.streamKey))
}
