package flv

import (
	"fmt"
	"playground/pkg/av"
)

type flvTag struct {
	TagType   uint8  // 1byte
	DataSize  uint32 // 3bytes
	TimeStamp uint32 // 3bytes
	//TimeStampExtended uint8  // 1byte
	StreamID uint32 // 3bytes
}

type mediaTag struct {
	/*
	 * soundFormat: UB[4]
	 * 0 = Linear PCM, platform endian
	 * 1 = ADPCM
	 * 2 = MP3
	 * 3 = Linear PCM, little endian
	 * 4 = Nellymoser 16-kHZ mono
	 * 5 = Nellymoser 8-kHZ mono
	 * 6 = Nellymoser
	 * 7 = G.711 A-law logarithmic PCM
	 * 8 = G.711 mu-law logarithmic PCM
	 * 9 = reserved
	 * 10 = AAC
	 * 11 = Speex
	 * 14 = MP3 8-kHZ
	 * 15 = Device-specific sound
	 * Formats 7, 8, 14, and 15 are reserved for internal use
	   AAC is supported in Flash Player 9,0,115,0 and higher.
	   Speex is supported in Flash Player 10 and higher.
	*/
	soundFormat uint8

	/*
	 * SoundRate: UB[2]
	 * 0 = 5.5-kHz For AAC: always 3
	 * 1 = 11-kHZ
	 * 2 = 22-kHZ
	 * 3 = 44-kHZ
	 */
	SoundRate uint8

	/*
	 * SoundSize: UB[1]
	 * 0 = snd8Bit
	 * 1 = snd16Bit
	 * Size of each sample.
	   This parameter only pertains to uncompressed formats.
	   Compressed formats always decode to 16 bits internally
	*/
	SoundSize uint8

	/*
	 * SoundType: UB[1]
	 * 0 = sndMono
	 * 1 = sndStereo
	 * Mono or stereo sound
	   For Nellymoser: always 0
	   For AAC: always 1
	*/
	SoundType uint8

	aacPacketType uint8 // 0 = AAC sequence header 1 = AAC raw

	/*
	 * 1: keyframe (for AVC, a seekable frame)
	 * 2: inter frame (for AVC, a non- seekable frame)
	 * 3: disposable inter frame (H.263 only)
	 * 4: generated keyframe (reserved for server use only)
	 * 5: video info/command frame
	 */
	FrameType uint8

	/*
	 * 1: JPEG (currently unused)
	 * 2: Sorenson H.263
	 * 3: Screen video
	 * 4: On2 VP6
	 * 5: On2 VP6 with alpha channel
	 * 6: Screen video version 2
	 * 7: AVC (H.264)
	 */
	codecID uint8

	/*
	 * 0: AVC sequence header
	 * 1: AVC NALU
	 * 2: AVC end of sequence (lower level NALU sequence ender is not required or supported)
	 */
	AvcPacketType uint8

	compositionTime int32
}

type Tag struct {
	flvTag   flvTag
	mediaTag mediaTag
}

// Audio CodecID
func (t *Tag) SoundFormat() uint8 {
	return t.mediaTag.soundFormat
}

// Audio AAC Packet Type. 0 = AAC sequence header 1 = AAC raw
func (t *Tag) AACPacketType() uint8 {
	return t.mediaTag.aacPacketType
}

func (t *Tag) IsKeyFrame() bool {
	return t.mediaTag.FrameType == av.KEY_FRAME
}

func (t *Tag) IsSeqHdr() bool {
	return t.IsKeyFrame() && t.mediaTag.AvcPacketType == av.AVC_SEQHDR
}

// Video CodecID
func (t *Tag) CodecID() uint8 {
	return t.mediaTag.codecID
}

func (t *Tag) CompositionTime() int32 {
	return t.mediaTag.compositionTime
}

func (t *Tag) decodeMediaTagHeader(b []byte, isVideo bool) (n int, err error) {
	if isVideo {
		return t.decodeVideoHeader(b)
	}

	return t.decodeAudioHeader(b)
}

func (t *Tag) decodeAudioHeader(b []byte) (n int, err error) {
	if len(b) < 1 {
		err = fmt.Errorf("invalid Audio Data len=%d", len(b))
		return
	}

	flags := b[0]
	t.mediaTag.soundFormat = flags >> 4
	t.mediaTag.SoundRate = (flags >> 2) & 0x3
	t.mediaTag.SoundSize = (flags >> 1) & 0x01
	t.mediaTag.SoundType = flags & 0x01
	n = 1

	switch t.mediaTag.soundFormat {
	case av.SOUND_AAC:
		t.mediaTag.aacPacketType = b[1]
		n++
	}

	return
}

func (t *Tag) decodeVideoHeader(b []byte) (n int, err error) {
	if len(b) < 5 {
		err = fmt.Errorf("invalid Video Data len=%d", len(b))
		return
	}

	flags := b[0]
	t.mediaTag.FrameType = flags >> 4
	t.mediaTag.codecID = flags & 0xf
	n = 1

	if t.mediaTag.FrameType == av.INTER_FRAME || t.mediaTag.FrameType == av.KEY_FRAME {
		t.mediaTag.AvcPacketType = b[1]
		for i := 2; i < 5; i++ {
			t.mediaTag.compositionTime = t.mediaTag.compositionTime<<8 + int32(b[i])
		}
		n += 4
	}

	return
}
