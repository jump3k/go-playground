package rtmp

import (
	"playground/pkg/av"
)

type SpecialCache struct {
	full bool
	pkt  *av.Packet
}

func NewSpecialCache() *SpecialCache {
	return &SpecialCache{}
}

func (c *SpecialCache) Write(pkt *av.Packet) {
	c.pkt = pkt
	c.full = true
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

func (c *Cache) Write(pkt *av.Packet) {
	if pkt.IsMetaData {
		c.metaData.Write(pkt)
		return
	} else {
		if !pkt.IsVideo {
			ah, ok := pkt.Header.(av.AudioPacketHeader)
			if ok {
				if ah.SoundFormat() == av.SOUND_AAC && ah.AACPacketType() == av.AAC_SEQHDR {
					c.audioSeq.Write(pkt)
					return
				} else {
					return
				}
			}
		} else {
			vh, ok := pkt.Header.(av.VideoPacketHeader)
			if ok {
				if vh.IsSeq() {
					c.videoSeq.Write(pkt)
					return
				}
			} else {
				return
			}
		}
	}

	//TODO: GOP cache
}