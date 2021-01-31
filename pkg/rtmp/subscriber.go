package rtmp

import (
	"github.com/go-kit/kit/log"
)

type subscriber struct {
	rtmpConn  *Conn
	streamKey string

	stopSub <-chan bool
	subType string // "gerneral"
	logger  log.Logger
}

func newSubscriber(c *Conn) *subscriber {
	sub := &subscriber{
		rtmpConn: c,
		subType:  "gerneral",
		logger:   c.logger,
		stopSub:  make(<-chan bool, 1),
	}

	sub.streamKey = genStreamKey(c.domain, c.appName, c.streamName)
	return sub
}
