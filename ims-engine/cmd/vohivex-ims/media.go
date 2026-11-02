package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
)

type callMedia struct {
	mu            sync.Mutex
	conn          net.PacketConn
	remote        *net.UDPAddr
	ip            net.IP
	pt            uint8
	codec         string
	id, dir       string
	cancel        context.CancelFunc
	once          sync.Once
	rx, tx        atomic.Uint64
	uplink        chan []int16
	downlink      chan []int16
	file          *os.File
	recordBytes   uint32
	recordPackets uint64
	recordErr     error
	recordStarted time.Time
	recordBase    [2]uint32
	recordOrigin  [2]uint32
	recordSeen    [2]bool
	ssrc          uint32
	seq           uint16
	timestamp     uint32
}

func newCallMedia(ctx context.Context, n *innerNetwork, id, dir string) (*callMedia, error) {
	c, e := n.listenUDP(0)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithCancel(ctx)
	m := &callMedia{conn: c, ip: n.ip, id: id, dir: dir, cancel: cancel, pt: 8, codec: "PCMA", uplink: make(chan []int16, 50), downlink: make(chan []int16, 50), ssrc: uint32(time.Now().UnixNano())}
	go m.read(ctx)
	go m.write(ctx)
	return m, nil
}
func (m *callMedia) offer() []byte {
	family := "IP4"
	if m.ip.To4() == nil {
		family = "IP6"
	}
	_, port, _ := net.SplitHostPort(m.conn.LocalAddr().String())
	return []byte(fmt.Sprintf("v=0\r\no=- 1 1 IN %s %s\r\ns=VoHiveX\r\nc=IN %s %s\r\nt=0 0\r\nm=audio %s RTP/AVP 8 0\r\na=rtpmap:8 PCMA/8000\r\na=rtpmap:0 PCMU/8000\r\na=ptime:20\r\na=sendrecv\r\n", family, m.ip, family, m.ip, port))
}
func (m *callMedia) negotiate(sdp []byte) error {
	var ip net.IP
	port := 0
	codec := ""
	pt := uint8(0)
	audio := false
	for _, line := range strings.Split(strings.ReplaceAll(string(sdp), "\r", ""), "\n") {
		fields := strings.Fields(line)
		if strings.HasPrefix(line, "c=IN ") && len(fields) >= 3 {
			ip = net.ParseIP(fields[2])
		}
		if strings.HasPrefix(line, "m=audio ") && len(fields) >= 4 {
			audio = true
			port, _ = strconv.Atoi(fields[1])
			if fields[2] != "RTP/AVP" {
				return errors.New("unsupported secure RTP profile")
			}
			for _, f := range fields[3:] {
				if f == "8" {
					pt, codec = 8, "PCMA"
					break
				}
				if f == "0" {
					pt, codec = 0, "PCMU"
					break
				}
			}
		}
	}
	if !audio || port < 1 || port > 65535 || ip == nil || codec == "" {
		return errors.New("operator did not negotiate a supported G.711 audio codec")
	}
	m.mu.Lock()
	m.remote = &net.UDPAddr{IP: ip, Port: port}
	m.pt, m.codec = pt, codec
	m.mu.Unlock()
	return nil
}
func (m *callMedia) read(ctx context.Context) {
	buf := make([]byte, 4096)
	for {
		n, from, e := m.conn.ReadFrom(buf)
		if e != nil {
			return
		}
		m.mu.Lock()
		remote, pt, codec := m.remote, m.pt, m.codec
		m.mu.Unlock()
		if remote == nil || from.String() != remote.String() {
			continue
		}
		var p rtp.Packet
		if p.Unmarshal(buf[:n]) != nil || p.PayloadType != pt || len(p.Payload) > 1600 {
			continue
		}
		samples := make([]int16, len(p.Payload))
		for i, v := range p.Payload {
			if codec == "PCMA" {
				samples[i] = decodeALaw(v)
			} else {
				samples[i] = decodeMuLaw(v)
			}
		}
		m.rx.Add(1)
		select {
		case m.downlink <- samples:
		default:
		}
		m.mu.Lock()
		m.record(samples, 1, p.Timestamp)
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}
func (m *callMedia) write(ctx context.Context) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			samples := make([]int16, 160)
			select {
			case input := <-m.uplink:
				copy(samples, input)
			default:
			}
			m.mu.Lock()
			remote, pt, codec := m.remote, m.pt, m.codec
			if remote == nil {
				m.mu.Unlock()
				continue
			}
			payload := make([]byte, 160)
			for i, v := range samples {
				if codec == "PCMA" {
					payload[i] = encodeALaw(v)
				} else {
					payload[i] = encodeMuLaw(v)
				}
			}
			p := rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: pt, SequenceNumber: m.seq, Timestamp: m.timestamp, SSRC: m.ssrc}, Payload: payload}
			m.seq++
			m.timestamp += 160
			wire, e := p.Marshal()
			if e == nil {
				_, e = m.conn.WriteTo(wire, remote)
			}
			if e == nil {
				m.tx.Add(1)
				m.record(samples, 0, p.Timestamp)
			}
			m.mu.Unlock()
		}
	}
}
func (m *callMedia) startRecording() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.file != nil {
		return errors.New("already recording")
	}
	if m.remote == nil {
		return errors.New("audio codec has not been negotiated")
	}
	if e := os.MkdirAll(m.dir, 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(m.dir, m.id+".wav.part"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(make([]byte, 44)); e != nil {
		f.Close()
		os.Remove(f.Name())
		return e
	}
	m.file = f
	m.recordBytes = 0
	m.recordPackets = 0
	m.recordErr = nil
	m.recordStarted = time.Now()
	m.recordSeen = [2]bool{}
	return nil
}

// A frame has two channels: transmitted audio on the left, received audio on
// the right. Recording preserves both directions; only genuine RTP counts.
func (m *callMedia) record(samples []int16, channel int, timestamp uint32) {
	if m.file == nil || m.recordErr != nil || channel < 0 || channel > 1 {
		return
	}
	if !m.recordSeen[channel] {
		m.recordSeen[channel] = true
		m.recordBase[channel] = timestamp
		m.recordOrigin[channel] = uint32(time.Since(m.recordStarted).Milliseconds()/20) * 160
	}
	delta := timestamp - m.recordBase[channel]
	// Reject implausible timestamps instead of allocating a huge sparse file.
	maxSamples := uint64(time.Since(m.recordStarted).Milliseconds()+1000) * 8
	position := uint64(m.recordOrigin[channel]) + uint64(delta)
	if position > maxSamples || (position+uint64(len(samples)))*4 > uint64(^uint32(0))-44 {
		return
	}
	b := make([]byte, len(samples)*4)
	_, _ = m.file.ReadAt(b, 44+int64(position*4))
	for i, v := range samples {
		binary.LittleEndian.PutUint16(b[i*4+channel*2:], uint16(v))
	}
	_, m.recordErr = m.file.WriteAt(b, 44+int64(position*4))
	end := uint32((position + uint64(len(samples))) * 4)
	if end > m.recordBytes {
		m.recordBytes = end
	}
	if channel == 1 {
		m.recordPackets++
	}
}
func (m *callMedia) stopRecording() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopRecordingLocked()
}
func (m *callMedia) stopRecordingLocked() (string, error) {
	f := m.file
	if f == nil {
		return "", errors.New("not recording")
	}
	m.file = nil
	defer f.Close()
	if m.recordErr != nil || m.recordPackets == 0 {
		os.Remove(f.Name())
		return "", errors.New("no received RTP audio was recorded")
	}
	h := wavHeader(m.recordBytes, 2, 8000)
	if _, e := f.WriteAt(h, 0); e != nil {
		os.Remove(f.Name())
		return "", e
	}
	if e := f.Sync(); e != nil {
		os.Remove(f.Name())
		return "", e
	}
	dest := strings.TrimSuffix(f.Name(), ".part")
	if e := os.Rename(f.Name(), dest); e != nil {
		return "", e
	}
	return dest, nil
}
func (m *callMedia) close() {
	m.once.Do(func() {
		m.cancel()
		m.conn.Close()
		m.mu.Lock()
		if m.file != nil {
			_, _ = m.stopRecordingLocked()
		}
		m.mu.Unlock()
	})
}
func wavHeader(size uint32, channels uint16, rate uint32) []byte {
	b := make([]byte, 44)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], 36+size)
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], channels)
	binary.LittleEndian.PutUint32(b[24:], rate)
	binary.LittleEndian.PutUint32(b[28:], rate*uint32(channels)*2)
	binary.LittleEndian.PutUint16(b[32:], channels*2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], size)
	return b
}
func decodeALaw(v byte) int16 {
	v ^= 0x55
	t := int(v&15) << 4
	seg := int((v & 0x70) >> 4)
	if seg == 0 {
		t += 8
	} else {
		t += 0x108
		if seg > 1 {
			t <<= seg - 1
		}
	}
	if v&0x80 == 0 {
		t = -t
	}
	return int16(t)
}
func decodeMuLaw(v byte) int16 {
	v = ^v
	t := ((int(v) & 15) << 3) + 0x84
	t <<= (v & 0x70) >> 4
	if v&0x80 != 0 {
		return int16(0x84 - t)
	}
	return int16(t - 0x84)
}
func encodeMuLaw(v int16) byte {
	x := int(v)
	mask := byte(0xff)
	if x < 0 {
		x = -x
		mask = 0x7f
	}
	if x > 32635 {
		x = 32635
	}
	x += 0x84
	seg := 0
	for t := x >> 7; t > 1 && seg < 7; t >>= 1 {
		seg++
	}
	return byte((seg<<4)|((x>>(seg+3))&15)) ^ mask
}
func encodeALaw(v int16) byte {
	x := int(v)
	mask := byte(0xd5)
	if x < 0 {
		x = -x - 1
		mask = 0x55
	}
	if x > 32767 {
		x = 32767
	}
	seg := 0
	for t := x >> 8; t > 0 && seg < 7; t >>= 1 {
		seg++
	}
	var low int
	if seg < 2 {
		low = (x >> 4) & 15
	} else {
		low = (x >> (seg + 3)) & 15
	}
	return byte(seg<<4|low) ^ mask
}
