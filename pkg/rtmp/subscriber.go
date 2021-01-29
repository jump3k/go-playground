package rtmp

import (
	"github.com/go-kit/kit/log"
)

type subscriber struct {
	rtmpConn  *Conn
	streamKey string

	subType string // "gerneral"
	logger  log.Logger
}

func newSubscriber(c *Conn) *subscriber {
	sub := &subscriber{
		rtmpConn: c,
		subType:  "gerneral",
		logger:   c.logger,
	}

	sub.streamKey = genStreamKey(c.domain, c.appName, c.streamName)
	return sub
}
