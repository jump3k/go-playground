package rtmp

import (
	"bufio"
	"io"
)

type readWriter struct {
	*bufio.Reader
	readError  error
	//writeError error
}

func newReadWriter(rw io.ReadWriter, readBufSize, writeBufSize int) *readWriter {
	return &readWriter{
		Reader: bufio.NewReaderSize(rw, readBufSize),
	}
}

func (rw *readWriter) Read(p []byte) (int, error) {
	if rw.readError != nil {
		return 0, rw.readError
	}

	nr, err := io.ReadAtLeast(rw.Reader, p, len(p))
	if err != nil {
		rw.readError = err
		return 0, err
	}

	return nr, nil
}

func (rw *readWriter) ReadError() error {
	return rw.readError
}

/*
func (rw *readWriter) WriteError() error {
	return rw.writeError
}

func (rw *readWriter) Flush() error {
	if rw.writeError != nil {
		return rw.writeError
	}

	if rw.ReadWriter.Writer.Buffered() == 0 {
		return nil
	}

	return rw.ReadWriter.Flush()
}*/
