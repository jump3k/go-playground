package flv

import "playground/pkg/av"

type Demuxer struct{}

func NewDemuxer() *Demuxer {
	return &Demuxer{}
}

func (dm *Demuxer) Demux(pkt *av.Packet) (*av.Packet, error) {
	//TODO:
	return pkt, nil
}

func (dm *Demuxer) DemuxHdr(pkt *av.Packet) error {
	var t *Tag
	_, err := t.decodeMediaTagHeader(pkt.Data, pkt.IsVideo)
	if err != nil {
		return err
	} else {
		pkt.Header = t
	}

	return nil
}
