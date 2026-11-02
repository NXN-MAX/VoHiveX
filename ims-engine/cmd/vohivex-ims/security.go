package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	vc "github.com/iniwex5/vowifi-go/runtimehost/voiceclient"
	"hash"
	"net"
	"sync"
)

// RFC 4303 transport-mode ESP, negotiated ipsec-3gpp AES-CBC/null encryption
// with SHA1/MD5 HMAC integrity, within the authenticated ePDG tunnel.
// Unadvertised algorithms are rejected, never silently downgraded.
type imsSecurity struct {
	mu            sync.Mutex
	local, remote net.IP
	ue, pc        vc.SecurityAgreement
	key           []byte
	hash          func() hash.Hash
	cipher        cipher.Block
	seq           map[uint32]uint32
	highest       map[uint32]uint32
	window        map[uint32]uint64
}

func newIMSSecurity(local, remote net.IP, ue, pc vc.SecurityAgreement, keys vc.IMSSecurityAKAKeys) (*imsSecurity, error) {
	if pc.Protocol != "ipsec-3gpp" || (pc.EncryptionAlgorithm != "null" && pc.EncryptionAlgorithm != "aes-cbc") || (pc.Algorithm != "hmac-sha-1-96" && pc.Algorithm != "hmac-md5-96") || len(keys.IK) != 16 {
		return nil, errors.New("unsupported IMS security agreement")
	}
	if ue.SPIClient == 0 || ue.SPIServer == 0 || pc.SPIClient == 0 || pc.SPIServer == 0 || pc.PortClient < 1 || pc.PortServer < 1 {
		return nil, errors.New("incomplete IMS security agreement")
	}
	// RFC 2104 SHA1 HMAC key is IK extended to 160 bits with zeros (TS 33.203).
	key := make([]byte, 20)
	copy(key, keys.IK)
	s := &imsSecurity{local: local, remote: remote, ue: ue, pc: pc, key: key, hash: sha1.New, seq: map[uint32]uint32{}, highest: map[uint32]uint32{}, window: map[uint32]uint64{}}
	if pc.Algorithm == "hmac-md5-96" {
		s.hash = md5.New
		s.key = s.key[:16]
	}
	if pc.EncryptionAlgorithm == "aes-cbc" {
		if len(keys.CK) != 16 {
			return nil, errors.New("IMS AES requires 128-bit CK")
		}
		var err error
		s.cipher, err = aes.NewCipher(keys.CK)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}
func ipParts(p []byte) (offset int, proto byte, src, dst net.IP, ok bool) {
	if len(p) < 20 {
		return
	}
	switch p[0] >> 4 {
	case 4:
		offset = int(p[0]&15) * 4
		if offset < 20 || offset > len(p) || int(binary.BigEndian.Uint16(p[2:4])) != len(p) || binary.BigEndian.Uint16(p[6:8])&0x3fff != 0 {
			return
		}
		proto = p[9]
		src = net.IP(p[12:16])
		dst = net.IP(p[16:20])
	case 6:
		if len(p) < 40 || int(binary.BigEndian.Uint16(p[4:6]))+40 != len(p) {
			return
		}
		offset = 40
		proto = p[6]
		src = net.IP(p[8:24])
		dst = net.IP(p[24:40])
	default:
		return
	}
	ok = true
	return
}
func replaceIPPayload(p []byte, offset int, proto byte, body []byte) []byte {
	out := append(append([]byte(nil), p[:offset]...), body...)
	if out[0]>>4 == 4 {
		out[9] = proto
		binary.BigEndian.PutUint16(out[2:4], uint16(len(out)))
		out[10], out[11] = 0, 0
		var sum uint32
		for i := 0; i < offset; i += 2 {
			sum += uint32(binary.BigEndian.Uint16(out[i : i+2]))
		}
		for sum>>16 != 0 {
			sum = (sum & 65535) + (sum >> 16)
		}
		binary.BigEndian.PutUint16(out[10:12], ^uint16(sum))
	} else {
		out[6] = proto
		binary.BigEndian.PutUint16(out[4:6], uint16(len(out)-40))
	}
	return out
}
func (s *imsSecurity) seal(p []byte) ([]byte, error) {
	off, proto, src, dst, ok := ipParts(p)
	if !ok {
		return nil, errors.New("invalid IP packet")
	}
	if !src.Equal(s.local) || !dst.Equal(s.remote) || (proto != 17 && proto != 6) || len(p) < off+4 {
		return p, nil
	}
	sport, dport := int(binary.BigEndian.Uint16(p[off:])), int(binary.BigEndian.Uint16(p[off+2:]))
	var spi uint32
	if sport == s.ue.PortClient && dport == s.pc.PortServer {
		spi = s.pc.SPIServer
	} else if sport == s.ue.PortServer && dport == s.pc.PortClient {
		spi = s.pc.SPIClient
	} else {
		return p, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq[spi]++
	if s.seq[spi] == 0 {
		return nil, errors.New("IMS ESP sequence exhausted")
	}
	body := make([]byte, 8)
	binary.BigEndian.PutUint32(body, spi)
	binary.BigEndian.PutUint32(body[4:], s.seq[spi])
	plain := append([]byte(nil), p[off:]...)
	alignment := 4
	if s.cipher != nil {
		alignment = s.cipher.BlockSize()
	}
	pad := (alignment - ((len(plain) + 2) % alignment)) % alignment
	for i := 1; i <= pad; i++ {
		plain = append(plain, byte(i))
	}
	plain = append(plain, byte(pad), proto)
	if s.cipher != nil {
		iv := make([]byte, s.cipher.BlockSize())
		if _, err := rand.Read(iv); err != nil {
			return nil, err
		}
		cipher.NewCBCEncrypter(s.cipher, iv).CryptBlocks(plain, plain)
		body = append(body, iv...)
	}
	body = append(body, plain...)
	mac := hmac.New(s.hash, s.key)
	mac.Write(body)
	body = append(body, mac.Sum(nil)[:12]...)
	return replaceIPPayload(p, off, 50, body), nil
}
func (s *imsSecurity) open(p []byte) ([]byte, error) {
	off, proto, src, dst, ok := ipParts(p)
	if !ok {
		return nil, errors.New("invalid IP packet")
	}
	if proto != 50 {
		if src.Equal(s.remote) && dst.Equal(s.local) && (proto == 6 || proto == 17) && len(p) >= off+4 {
			port := int(binary.BigEndian.Uint16(p[off+2:]))
			if port == s.ue.PortClient || port == s.ue.PortServer {
				return nil, errors.New("unprotected packet on negotiated IMS port")
			}
		}
		return p, nil
	}
	if !src.Equal(s.remote) || !dst.Equal(s.local) || len(p)-off < 24 {
		return nil, errors.New("unexpected IMS ESP packet")
	}
	body := p[off:]
	spi, seq := binary.BigEndian.Uint32(body), binary.BigEndian.Uint32(body[4:])
	if (spi != s.ue.SPIClient && spi != s.ue.SPIServer) || seq == 0 {
		return nil, errors.New("invalid IMS SPI")
	}
	signed := body[:len(body)-12]
	mac := hmac.New(s.hash, s.key)
	mac.Write(signed)
	if !hmac.Equal(mac.Sum(nil)[:12], body[len(body)-12:]) {
		return nil, errors.New("IMS ESP integrity rejected")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	high := s.highest[spi]
	if seq <= high {
		delta := high - seq
		if delta >= 64 || s.window[spi]&(1<<delta) != 0 {
			return nil, errors.New("IMS ESP replay rejected")
		}
		s.window[spi] |= 1 << delta
	} else {
		delta := seq - high
		if delta >= 64 {
			s.window[spi] = 1
		} else {
			s.window[spi] = (s.window[spi] << delta) | 1
		}
		s.highest[spi] = seq
	}
	plain := append([]byte(nil), signed[8:]...)
	if s.cipher != nil {
		block := s.cipher.BlockSize()
		if len(plain) < block*2 || len(plain)%block != 0 {
			return nil, errors.New("invalid IMS AES payload")
		}
		iv := plain[:block]
		plain = plain[block:]
		cipher.NewCBCDecrypter(s.cipher, iv).CryptBlocks(plain, plain)
	}
	if len(plain) < 2 {
		return nil, errors.New("short IMS ESP trailer")
	}
	pad, next := int(plain[len(plain)-2]), plain[len(plain)-1]
	end := len(plain) - 2 - pad
	if end < 0 || (next != 6 && next != 17) {
		return nil, errors.New("invalid IMS ESP trailer")
	}
	for i := 0; i < pad; i++ {
		if plain[end+i] != byte(i+1) {
			return nil, errors.New("invalid IMS ESP padding")
		}
	}
	payload := plain[:end]
	if len(payload) < 4 {
		return nil, errors.New("short IMS payload")
	}
	srcPort, dstPort := int(binary.BigEndian.Uint16(payload)), int(binary.BigEndian.Uint16(payload[2:]))
	if !((spi == s.ue.SPIClient && srcPort == s.pc.PortServer && dstPort == s.ue.PortClient) || (spi == s.ue.SPIServer && srcPort == s.pc.PortClient && dstPort == s.ue.PortServer)) {
		return nil, errors.New("IMS ESP port policy rejected")
	}
	return replaceIPPayload(p, off, next, payload), nil
}
