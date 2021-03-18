package rtmp

import (
	//"log"

	"bufio"
	"net"
	"os"

	"github.com/gwuhaolin/livego/protocol/amf"
	"github.com/sirupsen/logrus"
)

// Server returns a new RTMP server side conncetion
func Server(conn net.Conn, ssMgr *streamSourceMgr, config *Config) *Conn {
	c := &Conn{
		conn:   conn,
		ssMgr:  ssMgr,
		config: config,
	}
	c.handshakeFn = c.serverHandshake

	//TODO: config
	c.localChunksize = 60000
	c.remoteChunkSize = 128
	c.localWindowAckSize = 2500000
	c.remoteWindowAckSize = 250000

	//c.readWriter = newReadWriter(c, connReadBufSize, connWriteBufSize)
	c.reader = bufio.NewReader(conn)

	c.chunks = make(map[uint32]*ChunkStream)
	c.amfDecoder = &amf.Decoder{}
	c.amfEncoder = &amf.Encoder{}

	c.logger = config.Logger

	return c
}

// Client returns a new RTMP client side connection
func Client(conn net.Conn, config *Config) *Conn {
	c := &Conn{
		conn:     conn,
		config:   config,
		isClient: true,
	}
	c.handshakeFn = c.clientHandshake
	return c
}

type listener struct {
	net.Listener
	config *Config
	ssMgr  *streamSourceMgr // streamSourceMgr for every listener/server instance
}

func (l *listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	return Server(c, l.ssMgr, l.config), nil
}

func NewListener(inner net.Listener, config *Config) net.Listener {
	l := new(listener)
	l.Listener = inner
	l.ssMgr = newStreamSourceMgr()
	l.config = config
	return l
}

func Listen(network, laddr string, config *Config) (net.Listener, error) {
	l, err := net.Listen(network, laddr)
	if err != nil {
		return nil, err
	}
	return NewListener(l, config), nil
}

func ListenAndServe(network, laddr string, config *Config) error {
	logger := config.Logger.WithFields(logrus.Fields{
		"event": "ListenAndServe",
	})

	l, err := Listen(network, laddr, config)
	if err != nil {
		logger.Error(err)
		return err
	}

	logger.Tracef("listen at addr: %s, network: %s, pid: %d", l.Addr().String(), l.Addr().Network(), os.Getpid())

	for {
		conn, err := l.Accept()
		if err != nil {
			config.Logger.WithFields(logrus.Fields{"event": "Accept"}).Error(err)
			continue
		}

		go conn.(*Conn).Serve()
	}
}
