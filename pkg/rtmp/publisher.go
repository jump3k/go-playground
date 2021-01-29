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
		bs := make([]byte, 4096)
		if _, err := p.rtmpConn.Read(bs); err != nil {
			_ = p.logger.Log("level", "ERROR", "event", "recv publishing data", "error", err.Error())
			break
		}
	}
}

func (p *publisher) close() {
	p.pubMgr.deletePublisher(p.streamKey)
	_ = p.logger.Log("level", "INFO", "event", fmt.Sprintf("delete %s from streamMgr", p.streamKey))
}
