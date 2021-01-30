package av

type Demuxer interface {
	Demux(*Packet) (*Packet, error)
}