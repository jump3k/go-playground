package rtmp

import "playground/pkg/av"

type SpecialCache struct {
	//full bool
	pkt  *av.Packet
}

func NewSpecialCache() *SpecialCache {
	return &SpecialCache{}
}

type Cache struct {
	//gop
	videoSeq *SpecialCache
	audioSeq *SpecialCache
	metaData *SpecialCache
}

func NewCache() *Cache {
	return &Cache{
		videoSeq: NewSpecialCache(),
		audioSeq: NewSpecialCache(),
		metaData: NewSpecialCache(),
	}
}
