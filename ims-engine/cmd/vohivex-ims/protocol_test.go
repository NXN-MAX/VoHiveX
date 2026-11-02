package main

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	vc "github.com/iniwex5/vowifi-go/runtimehost/voiceclient"
)

func TestSIPInvitePRACKDoesNotBlock(t *testing.T) {
	server, _ := net.ListenPacket("udp", "127.0.0.1:0")
	defer server.Close()
	client, _ := net.ListenPacket("udp", "127.0.0.1:0")
	flow := newPacketSIPFlow(client, server.LocalAddr().(*net.UDPAddr))
	defer flow.close()
	fail := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		n, peer, err := server.ReadFrom(buf)
		if err != nil {
			fail <- err
			return
		}
		invite, err := vc.ParseSIPRequest(buf[:n])
		if err != nil {
			fail <- err
			return
		}
		p, _ := vc.BuildSIPResponseWire(invite, 183, "Session Progress", map[string]string{"Require": "100rel", "RSeq": "1"}, nil)
		server.WriteTo(p, peer)
		n, _, err = server.ReadFrom(buf)
		if err != nil {
			fail <- err
			return
		}
		prack, err := vc.ParseSIPRequest(buf[:n])
		if err != nil {
			fail <- err
			return
		}
		if prack.Method != "PRACK" {
			fail <- context.DeadlineExceeded
			return
		}
		ok, _ := vc.BuildSIPResponseWire(prack, 200, "OK", nil, nil)
		server.WriteTo(ok, peer)
		done, _ := vc.BuildSIPResponseWire(invite, 486, "Busy Here", nil, nil)
		server.WriteTo(done, peer)
		fail <- nil
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := flow.RoundTripInvite(ctx, vc.SIPRequestMessage{Method: "INVITE", URI: "sip:+12025550123@example.invalid", Headers: map[string]string{"Call-ID": "test", "CSeq": "1 INVITE", "From": "<sip:a@example.invalid>;tag=a", "To": "<sip:b@example.invalid>"}}, func(ctx context.Context, msg vc.SIPRequestMessage, res vc.SIPResponse) error {
		_, err := flow.RoundTripRequest(ctx, vc.SIPRequestMessage{Method: "PRACK", URI: msg.URI, Headers: map[string]string{"Call-ID": "test", "CSeq": "2 PRACK", "From": msg.Headers["From"], "To": msg.Headers["To"], "RAck": "1 1 INVITE"}})
		return err
	})
	if err != nil || res.StatusCode != 486 {
		t.Fatalf("PRACK blocked INVITE: %v %d", err, res.StatusCode)
	}
	if err := <-fail; err != nil {
		t.Fatal(err)
	}
}
func testPacket(src, dst net.IP, sport, dport uint16, payload []byte) []byte {
	b := make([]byte, 28+len(payload))
	b[0] = 0x45
	b[8] = 64
	b[9] = 17
	binary.BigEndian.PutUint16(b[2:], uint16(len(b)))
	copy(b[12:16], src.To4())
	copy(b[16:20], dst.To4())
	binary.BigEndian.PutUint16(b[20:], sport)
	binary.BigEndian.PutUint16(b[22:], dport)
	binary.BigEndian.PutUint16(b[24:], uint16(8+len(payload)))
	copy(b[28:], payload)
	return replaceIPPayload(b, 20, 17, b[20:])
}
func TestIMSSecurityFourAssociationsAndReplay(t *testing.T) {
	local, remote := net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2")
	ue, pc := vc.DefaultSecurityClientAgreement(nil), vc.DefaultSecurityClientAgreement(nil)
	ue.PortClient, ue.PortServer = 5062, 5063
	pc.PortClient, pc.PortServer = 51000, 51001
	keys := vc.IMSSecurityAKAKeys{IK: make([]byte, 16)}
	a, err := newIMSSecurity(local, remote, ue, pc, keys)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newIMSSecurity(remote, local, pc, ue, keys)
	for _, ports := range [][2]int{{ue.PortClient, pc.PortServer}, {ue.PortServer, pc.PortClient}} {
		plain := testPacket(local, remote, uint16(ports[0]), uint16(ports[1]), []byte("SIP/2.0"))
		sealed, err := a.seal(plain)
		if err != nil || sealed[9] != 50 {
			t.Fatalf("seal %v", err)
		}
		opened, err := b.open(sealed)
		if err != nil || string(opened) != string(plain) {
			t.Fatalf("open %v", err)
		}
		if _, err = b.open(sealed); err == nil {
			t.Fatal("replay accepted")
		}
		sealed[len(sealed)-1] ^= 1
		if _, err = b.open(sealed); err == nil {
			t.Fatal("bad MAC accepted")
		}
	}
}
func TestIMSSecurityRejectsDowngrade(t *testing.T) {
	ue, pc := vc.DefaultSecurityClientAgreement(nil), vc.DefaultSecurityClientAgreement(nil)
	pc.EncryptionAlgorithm = "unsupported-cipher"
	if _, err := newIMSSecurity(net.IPv4(1, 1, 1, 1), net.IPv4(2, 2, 2, 2), ue, pc, vc.IMSSecurityAKAKeys{IK: make([]byte, 16)}); err == nil {
		t.Fatal("unimplemented algorithm accepted")
	}
}
func TestRecordingKeepsDuplexTimeline(t *testing.T) {
	m := &callMedia{id: "test", dir: t.TempDir(), remote: &net.UDPAddr{IP: net.IPv4(192, 0, 2, 2), Port: 4000}}
	if err := m.startRecording(); err != nil {
		t.Fatal(err)
	}
	m.record([]int16{1, 2, 3, 4}, 0, 100)
	m.record([]int16{11, 12, 13, 14}, 1, 200)
	name, err := m.stopRecording()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 60 || binary.LittleEndian.Uint32(b[40:]) != 16 {
		t.Fatalf("duplex recording doubled duration: %d", len(b))
	}
	for i := 0; i < 4; i++ {
		if binary.LittleEndian.Uint16(b[44+i*4:]) != uint16(i+1) || binary.LittleEndian.Uint16(b[46+i*4:]) != uint16(i+11) {
			t.Fatal("channel erased")
		}
	}
	if st, _ := os.Stat(name); st.Mode().Perm() != 0600 {
		t.Fatal("recording permissions")
	}
}
func TestRecordingDoesNotSaveSilenceWithoutRTP(t *testing.T) {
	m := &callMedia{id: "test", dir: t.TempDir(), remote: &net.UDPAddr{Port: 4000}}
	if err := m.startRecording(); err != nil {
		t.Fatal(err)
	}
	m.record([]int16{0, 0}, 0, 100)
	if _, err := m.stopRecording(); err == nil {
		t.Fatal("empty call recorded as success")
	}
	files, _ := filepath.Glob(filepath.Join(m.dir, "*"))
	if len(files) != 0 {
		t.Fatal("empty file retained")
	}
}
func TestG711KnownSamples(t *testing.T) {
	if decodeALaw(0xd5) != 8 || decodeALaw(0x55) != -8 || decodeMuLaw(0xff) != 0 || decodeMuLaw(0x7f) != 0 {
		t.Fatal("G711 silence vectors")
	}
	for _, x := range []int16{-30000, -10000, -1000, -1, 0, 1, 1000, 10000, 30000} {
		for _, pair := range [][2]int16{{x, decodeALaw(encodeALaw(x))}, {x, decodeMuLaw(encodeMuLaw(x))}} {
			d := int(pair[0]) - int(pair[1])
			if d < 0 {
				d = -d
			}
			if d > 1024 {
				t.Fatalf("G711 roundtrip %v", pair)
			}
		}
	}
}
func TestSIPRejectsHeaderInjection(t *testing.T) {
	c, _ := net.ListenPacket("udp", "127.0.0.1:0")
	f := newPacketSIPFlow(c, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9})
	defer f.close()
	_, _, _, err := f.prepare(vc.SIPRequestMessage{Method: "INVITE", URI: "sip:a@example.invalid", Headers: map[string]string{"To": "x\r\nInjected: yes"}})
	if err == nil {
		t.Fatal("injected SIP header accepted")
	}
}
func TestSDPRejectsUnsupportedCodec(t *testing.T) {
	m := &callMedia{}
	err := m.negotiate([]byte("v=0\r\nc=IN IP4 192.0.2.2\r\nm=audio 4000 RTP/AVP 96\r\na=rtpmap:96 AMR/8000\r\n"))
	if err == nil || !strings.Contains(err.Error(), "supported") {
		t.Fatalf("unsupported audio advertised as ready: %v", err)
	}
}

func TestSIPStreamFragmentationAndCoalescing(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	a := "SIP/2.0 180 Ringing\r\nContent-Length: 3\r\n\r\nabc"
	b := "SIP/2.0 200 OK\r\nContent-Length: 0\r\n\r\n"
	go func() { server.Write([]byte("\r\n" + a[:15])); server.Write([]byte(a[15:] + b)) }()
	stream := newSIPStream(client)
	for _, want := range []string{a, b} {
		buf := make([]byte, 4096)
		n, _, err := stream.ReadFrom(buf)
		if err != nil || string(buf[:n]) != want {
			t.Fatalf("stream boundary: %v %q", err, buf[:n])
		}
	}
}
func TestIMSSecurityRejectsPlaintextSIP(t *testing.T) {
	ue, pc := vc.DefaultSecurityClientAgreement(nil), vc.DefaultSecurityClientAgreement(nil)
	local, remote := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
	s, err := newIMSSecurity(local, remote, ue, pc, vc.IMSSecurityAKAKeys{IK: make([]byte, 16)})
	if err != nil {
		t.Fatal(err)
	}
	for _, port := range []int{ue.PortClient, ue.PortServer} {
		p := testPacket(remote, local, uint16(pc.PortServer), uint16(port), []byte("forged SIP response"))
		if _, err := s.open(p); err == nil {
			t.Fatal("negotiated SIP port accepted plaintext")
		}
	}
	rtp := testPacket(remote, local, 35000, 35002, []byte("RTP"))
	if _, err := s.open(rtp); err != nil {
		t.Fatal("media incorrectly required SIP ESP", err)
	}
}

func TestIMSAESAndIntegrityAlgorithms(t *testing.T) {
	for _, alg := range []string{"hmac-sha-1-96", "hmac-md5-96"} {
		local, remote := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
		ue, pc := vc.DefaultSecurityClientAgreement(nil), vc.DefaultSecurityClientAgreement(nil)
		ue.Algorithm, pc.Algorithm = alg, alg
		ue.EncryptionAlgorithm, pc.EncryptionAlgorithm = "aes-cbc", "aes-cbc"
		keys := vc.IMSSecurityAKAKeys{IK: []byte("1234567890abcdef"), CK: []byte("fedcba0987654321")}
		a, err := newIMSSecurity(local, remote, ue, pc, keys)
		if err != nil {
			t.Fatal(err)
		}
		b, err := newIMSSecurity(remote, local, pc, ue, keys)
		if err != nil {
			t.Fatal(err)
		}
		plain := testPacket(local, remote, uint16(ue.PortClient), uint16(pc.PortServer), []byte("private REGISTER"))
		encrypted, err := a.seal(plain)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encrypted), "private REGISTER") {
			t.Fatal("AES payload remained plaintext")
		}
		result, err := b.open(encrypted)
		if err != nil || string(result) != string(plain) {
			t.Fatalf("%s: %v", alg, err)
		}
		if _, err := b.open(encrypted); err == nil {
			t.Fatal("encrypted replay accepted")
		}
	}
}

type radioFixture map[string]string

func (f radioFixture) ExecuteATSilent(q string, _ time.Duration) (string, error) { return f[q], nil }
func (f radioFixture) Close() error                                              { return nil }
func TestLocalUSBRequiresRadioAndPDPOff(t *testing.T) {
	for _, tc := range []struct {
		name, cfun, attach, pdp string
		ok                      bool
	}{
		{"safe", "+CFUN: 4", "+CGATT: 0", "+CGACT: 1,0\r\nOK", true},
		{"radio on", "+CFUN: 1", "+CGATT: 0", "OK", false},
		{"attached", "+CFUN: 4", "+CGATT: 1", "OK", false},
		{"active data", "+CFUN: 4", "+CGATT: 0", "+CGACT: 1,1\r\nOK", false},
		{"unreadable data", "+CFUN: 4", "+CGATT: 0", "ERROR", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &coreClient{local: radioFixture{"AT+CFUN?": tc.cfun, "AT+CGATT?": tc.attach, "AT+CGACT?": tc.pdp}}
			if err := c.checkOwnership(context.Background()); (err == nil) != tc.ok {
				t.Fatalf("ownership guard: %v", err)
			}
		})
	}
}
