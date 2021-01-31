package rtmp

import (
	"sync"
)

type streamSource struct {
	publishing <-chan bool
	pub        *publisher

	subs      map[string]*subscriber
	addSubMux sync.Mutex
}

func newStreamSource(pub *publisher) *streamSource {
	ss := &streamSource{
		publishing: make(<-chan bool, 1),
		pub:        pub,
		subs:       make(map[string]*subscriber),
	}

	return ss
}

func (ss *streamSource) setPublisher(pub *publisher) *streamSource {
	ss.pub = pub
	return ss
}

func (ss *streamSource) addSubscriber(sub *subscriber) bool {
	ss.addSubMux.Lock()
	defer ss.addSubMux.Unlock()

	if _, ok := ss.subs[sub.rtmpConn.RemoteAddr().String()]; ok { //exists
		return false
	}

	return true
}

func (ss *streamSource) delSubscriber(sub *subscriber) bool {
	ss.addSubMux.Lock()
	defer ss.addSubMux.Unlock()

	delete(ss.subs, sub.rtmpConn.RemoteAddr().String())
	return true
}

type streamSourceMgr struct {
	streamMap sync.Map //<StreamKey, StreamSource>
}

func newStreamSourceMgr() *streamSourceMgr {
	mgr := &streamSourceMgr{}

	return mgr
}
