package av

type PacketHeader interface{}

type AudioPacketHeader interface {
	PacketHeader
	SoundFormat() uint8
	AACPacketType() uint8
}

type VideoPacketHeader interface {
	PacketHeader
	IsKeyFrame() bool
	IsSeq() bool
	CodecID() uint8
	CompositionTime() int32
}

type Packet struct {
	Header PacketHeader
	Data   []byte

	TimeStamp uint32 // dts
	StreamID  uint32

	IsAudio    bool
	IsVideo    bool
	IsMetaData bool
}