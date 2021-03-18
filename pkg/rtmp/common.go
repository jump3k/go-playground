package rtmp

import (
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

type Config struct {
	Logger *logrus.Logger
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

const (
	cmdConnect       = "connect"
	cmdFcpublish     = "FCPublish"
	cmdReleaseStream = "releaseStream"
	cmdCreateStream  = "createStream"
	cmdPublish       = "publish"
	cmdFCUnpublish   = "FCUnpublish"
	cmdDeleteStream  = "deleteStream"
	cmdPlay          = "play"
)

const (
	streamBegin uint32 = 0
	//streamEOF        uint32 = 1
	//streamDry        uint32 = 2
	//setBufferLen     uint32 = 3
	streamIsRecorded uint32 = 4
	//pingRequest      uint32 = 6
	//pingResponse     uint32 = 7
)
