package main

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

// SIP over TCP is a framed byte stream; one Read is not necessarily one SIP
// message. Keep the same transaction dispatcher for UDP and reliable streams.
type sipStream struct {
	net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
}

func newSIPStream(c net.Conn) *sipStream {
	return &sipStream{Conn: c, reader: bufio.NewReaderSize(c, 4096)}
}
func (s *sipStream) ReadFrom(dst []byte) (int, net.Addr, error) {
	var h []byte
	length := 0
	seenLength := false
	for {
		part, err := s.reader.ReadSlice('\n')
		if err != nil {
			return 0, nil, err
		}
		line := string(part)
		if len(h) == 0 && strings.TrimSpace(line) == "" {
			continue
		}
		if len(h)+len(line) > 32768 {
			return 0, nil, errors.New("SIP header too large")
		}
		h = append(h, line...)
		if line == "\r\n" || line == "\n" {
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if ok && (strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "l")) {
			n, e := strconv.Atoi(strings.TrimSpace(v))
			if e != nil || n < 0 || n > 32768 || seenLength {
				return 0, nil, errors.New("invalid SIP Content-Length")
			}
			seenLength = true
			length = n
		}
	}
	if !seenLength {
		return 0, nil, errors.New("SIP TCP Content-Length missing")
	}
	if len(h)+length > len(dst) {
		return 0, nil, io.ErrShortBuffer
	}
	copy(dst, h)
	if _, err := io.ReadFull(s.reader, dst[len(h):len(h)+length]); err != nil {
		return 0, nil, err
	}
	peer := s.RemoteAddr().(*net.TCPAddr)
	return len(h) + length, &net.UDPAddr{IP: peer.IP, Port: peer.Port}, nil
}
func (s *sipStream) WriteTo(b []byte, peer net.Addr) (int, error) {
	if peer.String() != s.RemoteAddr().String() {
		return 0, errors.New("SIP TCP peer mismatch")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n := 0
	for n < len(b) {
		k, e := s.Write(b[n:])
		n += k
		if e != nil {
			return n, e
		}
		if k == 0 {
			return n, io.ErrShortWrite
		}
	}
	return n, nil
}
