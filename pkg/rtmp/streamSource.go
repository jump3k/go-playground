package rtmp

import (
	"playground/pkg/av"
	"sync"
	"time"
)

type streamSource struct {
	stopPublish chan bool
	publisher   *publisher

	subscribers     map[string]*subscriber
	subscriberCount int
	addSubMux       sync.Mutex

	streamKey string
	sessionID string
	ssMgr     *streamSourceMgr
	cache     *Cache
}

func newStreamSource(pub *publisher, streamKey string, ssMgr *streamSourceMgr) *streamSource {
	ss := &streamSource{
		stopPublish: make(chan bool, 1),
		publisher:   pub,
		subscribers: make(map[string]*subscriber),
		streamKey:   streamKey,
		sessionID:   genUuid(),
		ssMgr:       ssMgr,
		cache:       NewCache(),
	}

	return ss
}

func (ss *streamSource) doPublishing() error {
	err := ss.publisher.publishingCycle(ss)
	return err
}

func (ss *streamSource) doPlaying(sub *subscriber) error {
	err := sub.playingCycle(ss)
	return err
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

	if _, ok := ss.subscribers[sub.rtmpConn.RemoteAddr().String()]; ok { //exists
		return false
	}

	ss.subscribers[sub.rtmpConn.RemoteAddr().String()] = sub
	ss.subscriberCount++

	return true
}

func (ss *streamSource) delSubscriber(sub *subscriber) bool {
	ss.addSubMux.Lock()
	defer ss.addSubMux.Unlock()

	delete(ss.subscribers, sub.rtmpConn.RemoteAddr().String())
	return true
}

func (ss *streamSource) dispatchAvPkt(cs *ChunkStream, pkt *av.Packet) {
	for _, sub := range ss.subscribers {
		if sub.stopped {
			continue
		}

		if !sub.cacheSend {
			sub.cacheSend = true
			if err := ss.sendCache(sub); err != nil {
				continue
			}
		}

		sub.avPktEnQueue(pkt)
		sub.calcBaseTimeStamp()
	}
}

func (ss *streamSource) sendCache(sub *subscriber) error {
	metaData := ss.cache.metaData
	if metaData.full && metaData.pkt != nil {
		sub.avPktEnQueue(metaData.pkt)
	}

	videoSeq := ss.cache.videoSeq
	if videoSeq.full && videoSeq.pkt != nil {
		sub.avPktEnQueue(videoSeq.pkt)
	}

	audioSeq := ss.cache.audioSeq
	if audioSeq.full && audioSeq.pkt != nil {
		sub.avPktEnQueue(audioSeq.pkt)
	}

	//TODO: GOP

	return nil
}


type streamSourceMgr struct {
	streamMap sync.Map //<StreamKey, StreamSource>
}

func newStreamSourceMgr() *streamSourceMgr {
	mgr := &streamSourceMgr{}

	return mgr
}
