package rtmp

import "sync"

type publisherMgr struct {
	streamMap sync.Map //<streamKey, *publisher>
}

func newPublisherMgr() *publisherMgr {
	mgr := &publisherMgr{}

	return mgr
}

func (mgr *publisherMgr) storePublisher(streamKey string, pub *publisher) (*publisher, bool) {
	if actual, loaded := mgr.streamMap.LoadOrStore(streamKey, pub); loaded {
		return actual.(*publisher), true
	}

	return pub, false
}

func (mgr *publisherMgr) deletePublisher(streamKey string) {
	mgr.streamMap.Delete(streamKey)
}

/*
func (mgr *publisherMgr) existStream(streamKey string) bool {
	if _, ok := mgr.streamMap.Load(streamKey); ok {
		return true
	}

	return false
}
*/
