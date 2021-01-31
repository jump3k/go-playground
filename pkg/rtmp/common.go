package rtmp

import (
	"github.com/go-kit/kit/log"
	uuid "github.com/satori/go.uuid"
)

type Config struct {
	logger log.Logger

	connReadBufSize  int
	connWriteBufSize int
}

type ConnectionState struct {
	HandshakeComplete bool
	Vhost             string
}

func genStreamKey(domain, app, stream string) string {
	return domain + "/" + app + "/" + stream
}

func genUuid() string {
	return uuid.NewV4().String()
}
