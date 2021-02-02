package rtmp

import (
	"sync"
	"time"
)

type streamSource struct {
	stopPublish chan bool
	publisher   *publisher

	subs      map[string]*subscriber
	addSubMux sync.Mutex

	streamKey string
	ssMgr     *streamSourceMgr
}

func newStreamSource(pub *publisher) *streamSource {
	ss := &streamSource{
		stopPublish: make(chan bool, 1),
		publisher:   pub,
		subs:        make(map[string]*subscriber),
		streamKey:   pub.streamKey,
		ssMgr:       pub.ssMgr,
	}

	return ss
}

func (ss *streamSource) doPublishing() error {
	err := ss.publisher.publishingCycle()
	return err
}

func (ss *streamSource) doPlaying() error {
	//TODO:
	return nil
}

func (ss *streamSource) setPublisher(pub *publisher) *streamSource {
	ss.publisher = pub
	return ss
}

func (ss *streamSource) delPublisher() {
	ss.publisher = nil

	time.AfterFunc(time.Minute, func() {
		val, ok := ss.ssMgr.streamMap.Load(ss.streamKey)
		if ok {
			ssCache := val.(*streamSource)
			if ssCache.publisher == nil {
				ss.ssMgr.streamMap.Delete(ss.streamKey)
				ss.stopPublish <- true
			}
		}
	})
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
