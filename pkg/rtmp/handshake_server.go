package rtmp

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand"
)

func (c *Conn) serverHandshake() error {
	/* random:
	1. c0c1c2: c0(1) + c1(1536) + c2(1536)
	2. s0s1s2: s0(1) + s1(1536) + c2(1536)
	*/
	var random [(1 + 1536*2) * 2]byte

	c0c1c2 := random[:1536*2+1]
	s0s1s2 := random[1536*2+1:]

	c0 := c0c1c2[:1]
	c1 := c0c1c2[1 : 1536+1]
	c0c1 := c0c1c2[:1536+1]
	c2 := c0c1c2[1536+1:]

	s0 := s0s1s2[:1]
	s1 := s0s1s2[1 : 1536+1]
	s0s1 := s0s1s2[:1536+1]
	s2 := s0s1s2[1536+1:]

	c.logger.WithField("event", "start to server handshake...").Info("")

	// read c0c1
	if _, err := io.ReadFull(c.readWriter, c0c1); err != nil {
		c.logger.WithField("event", "read c0c1").Error(err)
		return err
	}
	c.logger.WithField("event", "read c0c1").Info("success")

	if c0[0] != 3 {
		return fmt.Errorf("rtmp: handshake version=%d invalid", c0c1[0])
	}

	cliVer := byteSliceAsUint(c1[4:8], true)
	if cliVer != 0 {
		cliTime := byteSliceAsUint(c1[0:4], true)
		srvTime, srvVer := cliTime, uint32(0x0d0e0a0d)

		if ok, digest := handshakeParse1(c1, hsClientPartialKey, hsServerFullKey); !ok {
			return fmt.Errorf("rtmp: handshake server: C1 invalid")
		} else {
			handshakeCreate01(s0s1, srvTime, srvVer, hsServerPartialKey)
			handshakeCreate2(s2, digest)
		}
		c.logger.WithField("event", "complex handshake").Info("")
	} else {
		s0[0] = 3
		copy(s1, c2)
		copy(s2, c1)
		c.logger.WithField("event", "simple handshake").Info("")
	}

	// write s0s1s2
	if _, err := c.readWriter.Write(s0s1s2); err != nil {
		c.logger.WithField("event", "write s0s1s2").Error(err)
		return err
	}

	if err := c.readWriter.Flush(); err != nil {
		c.logger.WithField("event", "flush s0s1s2").Error(err)
		return err
	}
	c.logger.WithField("event", "flush s0s1s2").Info("")

	// read c2
	if _, err := io.ReadFull(c, c2); err != nil {
		c.logger.WithField("event", "read c2").Error(err)
		return err
	}
	c.logger.WithField("event", "read c2").Info("")

	return nil
}

func handshakeParse1(p []byte, peerKey []byte, key []byte) (ok bool, digest []byte) {
	var pos int
	if pos := handshakeFindDigest(p, peerKey, 772); pos == -1 {
		if pos = handshakeFindDigest(p, peerKey, 8); pos == -1 {
			return
		}
	}

	ok = true
	digest = handshakeMakeDigest(key, p[pos:pos+32], -1)
	return
}

func handshakeFindDigest(p []byte, key []byte, base int) int {
	gap := handshakeCalcDigestPos(p, base)
	digest := handshakeMakeDigest(key, p, gap)

	if !bytes.Equal(p[gap:gap+32], digest) {
		return -1
	}

	return gap
}

func handshakeCalcDigestPos(p []byte, base int) (pos int) {
	for i := 0; i < 4; i++ {
		pos += int(p[base+i])
	}

	pos = (pos % 728) + base + 4
	return
}

func handshakeMakeDigest(key []byte, src []byte, gap int) []byte {
	h := hmac.New(sha256.New, key)
	if gap <= 0 {
		_, _ = h.Write(src)
	} else {
		_, _ = h.Write(src[:gap])
		_, _ = h.Write(src[gap+32:])
	}
	return h.Sum(nil)
}

func handshakeCreate01(p []byte, time uint32, ver uint32, key []byte) {
	p[0] = 3
	p1 := p[1:]
	rand.Read(p1[8:])

	uintAsbyteSlice(time, p1[0:4], true)
	uintAsbyteSlice(ver, p1[1:8], true)

	gap := handshakeCalcDigestPos(p1, 8)
	digest := handshakeMakeDigest(key, p1, gap)
	copy(p1[gap:], digest)
}

func handshakeCreate2(p, key []byte) {
	rand.Read(p)
	gap := len(p) - 32
	digest := handshakeMakeDigest(key, p, gap)
	copy(p[gap:], digest)
}

var (
	hsClientFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'P', 'l', 'a', 'y', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsServerFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'M', 'e', 'd', 'i', 'a', ' ',
		'S', 'e', 'r', 'v', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsClientPartialKey = hsClientFullKey[:30]
	hsServerPartialKey = hsServerFullKey[:36]
)
