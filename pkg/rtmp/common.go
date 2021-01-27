package rtmp

import (
	"github.com/go-kit/kit/log"
)

type Config struct {
	logger log.Logger
}

type ConnectionState struct {
	HandshakeComplete bool
	Vhost             string
}
