package rtmp

func (c *Conn) serverHandshake() error {
	/*
		var random [(1 + 1536*2) * 2]byte

		c0c1c2 := random[:1536*2+1]
		c0 := c0c1c2[:1]
		c1 := c0c1c2[1 : 1536+1]
		c0c1 := c0c1c2[:1536+1]
		c2 := c0c1c2[1536+1:]

		s0s1s2 := random[1536*2+1:]
		s0 := s0s1s2[:1]
		s1 := s0s1s2[1 : 1536+1]
		s0s1 := s0s1s2[:1536+1]
		s2 := s0s1s2[1536+1:]
	*/

	return nil
}
