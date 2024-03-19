package shell

import (
	"bytes"
	"io"
)

type multilineReader struct {
	lines   []Line
	i       int
	current io.Reader
}

func NewMultilineReader(lines []Line) *multilineReader {
	var current io.Reader
	if len(lines) > 0 {
		current = bytes.NewReader(lines[0])
	} else {
		current = bytes.NewReader([]byte{})
	}
	return &multilineReader{
		lines:   lines,
		i:       0,
		current: current,
	}
}

func (multilinereader *multilineReader) next() error {
	if multilinereader.i+1 < len(multilinereader.lines) {
		multilinereader.i += 1
		multilinereader.current = bytes.NewReader(multilinereader.lines[multilinereader.i])
		return nil
	}

	return io.EOF
}

func (multilinereader *multilineReader) Read(p []byte) (int, error) {
	n, err := multilinereader.current.Read(p)
	if err == io.EOF && multilinereader.next() == nil {
		n1, err := multilinereader.Read(p[n:])
		return n + n1, err
	}
	return n, err
}
