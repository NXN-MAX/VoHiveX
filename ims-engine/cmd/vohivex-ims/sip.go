package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	vc "github.com/iniwex5/vowifi-go/runtimehost/voiceclient"
)

func randomID() string { var b [12]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func header(h map[string][]string, k string) string {
	for name, v := range h {
		if strings.EqualFold(k, name) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}
func stringHeader(h map[string]string, k string) string {
	for name, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

type receivedSIP struct {
	response vc.SIPResponse
	err      error
}
type sipFlow struct {
	transport           string
	mu                  sync.Mutex
	conn                net.PacketConn
	remote              *net.UDPAddr
	pending             map[string]chan receivedSIP
	closed              chan struct{}
	once                sync.Once
	sockets             []net.PacketConn
	onRequest           func(vc.SIPIncomingRequest, func(int, string, map[string]string, []byte))
	onProvisional       func(vc.SIPRequestMessage, vc.SIPResponse)
	network             *innerNetwork
	security            *imsSecurity
	localAgreement      vc.SecurityAgreement
	instance            string
	privateID, realm    string
	registrationFailure string
}

func newSIPFlow(ctx context.Context, n *innerNetwork, remote *net.UDPAddr, transport string) (*sipFlow, error) {
	var c net.PacketConn
	var e error
	if transport == "tcp" {
		c, e = n.dialTCP(ctx, 5060, remote)
	} else {
		c, e = n.listenUDP(5060)
	}
	if e != nil {
		return nil, e
	}
	f := newPacketSIPFlow(c, remote)
	f.network = n
	f.transport = transport
	return f, nil
}
func newPacketSIPFlow(c net.PacketConn, remote *net.UDPAddr) *sipFlow {
	f := &sipFlow{transport: "udp", conn: c, remote: remote, pending: map[string]chan receivedSIP{}, closed: make(chan struct{}), sockets: []net.PacketConn{c}}
	go f.receive(c)
	return f
}
func transactionKey(headers map[string][]string) string {
	return header(headers, "Call-ID") + "\n" + strings.ToUpper(header(headers, "CSeq"))
}
func requestKey(m vc.SIPRequestMessage) string {
	return stringHeader(m.Headers, "Call-ID") + "\n" + strings.ToUpper(stringHeader(m.Headers, "CSeq"))
}
func (f *sipFlow) receive(c net.PacketConn) {
	buf := make([]byte, 65535)
	for {
		size, from, e := c.ReadFrom(buf)
		if e != nil {
			return
		}
		sender, ok := from.(*net.UDPAddr)
		f.mu.Lock()
		peerIP := append(net.IP(nil), f.remote.IP...)
		f.mu.Unlock()
		if !ok || !sender.IP.Equal(peerIP) {
			continue
		}
		raw := append([]byte(nil), buf[:size]...)
		if strings.TrimSpace(string(raw)) == "" {
			continue
		}
		if strings.HasPrefix(string(raw), "SIP/2.0") {
			response, e := vc.ParseSIPResponse(raw)
			if e != nil {
				continue
			}
			key := transactionKey(response.Headers)
			f.mu.Lock()
			ch := f.pending[key]
			f.mu.Unlock()
			if ch != nil {
				select {
				case ch <- receivedSIP{response: response}:
				case <-f.closed:
					return
				default:
				}
			}
		} else {
			request, e := vc.ParseSIPRequest(raw)
			if e != nil {
				continue
			}
			reply := func(code int, reason string, h map[string]string, body []byte) {
				wire, e := vc.BuildSIPResponseWire(request, code, reason, h, body)
				if e == nil {
					_, _ = c.WriteTo(wire, from)
				}
			}
			f.mu.Lock()
			handler := f.onRequest
			f.mu.Unlock()
			if handler != nil {
				go handler(request, reply)
			} else if request.Method != "ACK" {
				reply(503, "Service Unavailable", nil, nil)
			}
		}
	}
}
func (f *sipFlow) close() {
	f.once.Do(func() {
		close(f.closed)
		f.mu.Lock()
		cs := append([]net.PacketConn(nil), f.sockets...)
		f.mu.Unlock()
		for _, c := range cs {
			_ = c.Close()
		}
	})
}
func (f *sipFlow) RoundTripRegister(ctx context.Context, m vc.RegisterMessage) (vc.RegisterResponse, error) {
	h := make(map[string]string, len(m.Headers))
	for k, v := range m.Headers {
		h[k] = v
	}
	f.mu.Lock()
	if f.instance == "" {
		f.instance = "urn:uuid:" + uuid.NewString()
	}
	instance := f.instance
	f.mu.Unlock()
	h["Proxy-Require"] = "sec-agree"
	h["Supported"] = "path,sec-agree,gruu"
	f.mu.Lock()
	proxy := f.remote.IP.String()
	f.mu.Unlock()
	if strings.Contains(proxy, ":") {
		proxy = "[" + proxy + "]"
	}
	h["Route"] = "<sip:" + proxy + ";lr>"
	h["P-Visited-Network-ID"] = `"` + f.realm + `"`
	h["P-Access-Network-Info"] = "IEEE-802.11;i-wlan-node-id=000000000000"
	h["Contact"] = strings.ReplaceAll(h["Contact"], "urn:uuid:vowifi-go", instance) + `;+g.3gpp.smsip;audio;+g.3gpp.accesstype="IEEE-802.11";expires=` + h["Expires"]
	if stringHeader(h, "Authorization") == "" && f.privateID != "" {
		h["Authorization"] = fmt.Sprintf(`Digest uri="%s",username="%s",algorithm=AKAv1-MD5,response="",realm="%s",nonce=""`, m.URI, f.privateID, f.realm)
	}
	if os.Getenv("VOHIVEX_IMS_TRACE_HEADERS") == "1" {
		// Never output authorization, subscriber identities, or AKA material.
		digits := regexp.MustCompile(`[0-9]{6,}`)
		keys := make([]string, 0, len(h))
		for k := range h {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := h[k]
			if strings.Contains(strings.ToLower(k), "authorization") {
				v = "[redacted]"
			}
			fmt.Fprintf(os.Stderr, "%s: %q\n", k, digits.ReplaceAllString(v, "[digits]"))
		}
	}
	res, err := f.roundTrip(ctx, vc.SIPRequestMessage{Method: "REGISTER", URI: m.URI, Headers: h, Body: m.Body}, nil)
	if res.StatusCode >= 400 {
		f.mu.Lock()
		f.registrationFailure = fmt.Sprintf("status=%d warning=%s reason=%s server=%s require=%s unsupported=%s", res.StatusCode, header(res.Headers, "Warning"), header(res.Headers, "Reason"), header(res.Headers, "Server"), header(res.Headers, "Require"), header(res.Headers, "Unsupported"))
		f.mu.Unlock()
	}
	return res, err
}
func (f *sipFlow) RoundTripRequest(ctx context.Context, m vc.SIPRequestMessage) (vc.SIPResponse, error) {
	return f.roundTrip(ctx, m, nil)
}
func (f *sipFlow) RoundTripInvite(ctx context.Context, m vc.SIPRequestMessage, on vc.ProvisionalResponseHandler) (vc.SIPResponse, error) {
	return f.roundTrip(ctx, m, on)
}
func (f *sipFlow) prepare(m vc.SIPRequestMessage) ([]byte, net.PacketConn, *net.UDPAddr, error) {
	f.mu.Lock()
	c, r := f.conn, *f.remote
	f.mu.Unlock()
	h := make(map[string]string, len(m.Headers)+3)
	for k, v := range m.Headers {
		if strings.ContainsAny(k+v, "\r\n") {
			return nil, nil, nil, errors.New("invalid SIP header")
		}
		h[k] = v
	}
	if stringHeader(h, "Via") == "" {
		h["Via"] = "SIP/2.0/" + strings.ToUpper(f.transport) + " " + c.LocalAddr().String() + ";branch=z9hG4bK" + randomID() + ";rport"
	}
	if stringHeader(h, "Max-Forwards") == "" {
		h["Max-Forwards"] = "70"
	}
	h["Content-Length"] = strconv.Itoa(len(m.Body))
	if strings.ContainsAny(m.URI+m.Method, "\r\n") {
		return nil, nil, nil, errors.New("invalid SIP request")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s SIP/2.0\r\n", m.Method, m.URI)
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Place routing/transaction headers first as recommended by RFC 3261
	// section 7.3.1, rather than relying on alphabetical header ordering.
	priority := map[string]int{"via": 0, "route": 1, "max-forwards": 2, "from": 3, "to": 4, "call-id": 5, "cseq": 6, "contact": 7}
	sort.SliceStable(keys, func(i, j int) bool {
		a, okA := priority[strings.ToLower(keys[i])]
		b, okB := priority[strings.ToLower(keys[j])]
		if !okA {
			a = 20
		}
		if !okB {
			b = 20
		}
		return a < b
	})
	for _, k := range keys {
		fmt.Fprintf(&b, "%s: %s\r\n", k, h[k])
	}
	b.WriteString("\r\n")
	b.Write(m.Body)
	return []byte(b.String()), c, &r, nil
}
func (f *sipFlow) WriteRequest(ctx context.Context, m vc.SIPRequestMessage) error {
	wire, c, r, e := f.prepare(m)
	if e != nil {
		return e
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.closed:
		return errors.New("IMS transport closed")
	default:
	}
	_, e = c.WriteTo(wire, r)
	return e
}
func (f *sipFlow) roundTrip(ctx context.Context, m vc.SIPRequestMessage, on vc.ProvisionalResponseHandler) (vc.SIPResponse, error) {
	wire, c, r, e := f.prepare(m)
	if e != nil {
		return vc.SIPResponse{}, e
	}
	key := requestKey(m)
	ch := make(chan receivedSIP, 32)
	f.mu.Lock()
	if _, exists := f.pending[key]; exists {
		f.mu.Unlock()
		return vc.SIPResponse{}, errors.New("duplicate SIP transaction")
	}
	f.pending[key] = ch
	f.mu.Unlock()
	defer func() { f.mu.Lock(); delete(f.pending, key); f.mu.Unlock() }()
	if _, e = c.WriteTo(wire, r); e != nil {
		return vc.SIPResponse{}, e
	}
	interval := 500 * time.Millisecond
	timer := time.NewTimer(interval)
	defer timer.Stop()
	provisional := false
	timeout := 32 * time.Second
	if m.Method == "INVITE" {
		timeout = 180 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	seen := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			return vc.SIPResponse{}, ctx.Err()
		case <-f.closed:
			return vc.SIPResponse{}, errors.New("IMS transport closed")
		case <-deadline.C:
			return vc.SIPResponse{}, vc.ErrSIPFinalResponseTimeout
		case <-timer.C:
			if f.transport != "tcp" && (!provisional || m.Method != "INVITE") {
				if _, e = c.WriteTo(wire, r); e != nil {
					return vc.SIPResponse{}, e
				}
				interval *= 2
				if interval > 4*time.Second {
					interval = 4 * time.Second
				}
				timer.Reset(interval)
			}
		case item := <-ch:
			res := item.response
			if res.StatusCode < 200 {
				provisional = true
				k := fmt.Sprintf("%d:%s:%s", res.StatusCode, header(res.Headers, "RSeq"), header(res.Headers, "To"))
				if !seen[k] {
					seen[k] = true
					f.mu.Lock()
					observer := f.onProvisional
					f.mu.Unlock()
					if observer != nil {
						observer(m, res)
					}
					if on != nil {
						if e := on(ctx, m, res); e != nil {
							return vc.SIPResponse{}, e
						}
					}
				}
				continue
			}
			return res, nil
		}
	}
}

func (f *sipFlow) InstallSecurityPlan(ctx context.Context, p vc.IMSSecurityAssociationPlan) error {
	return errors.New("IMS security requires AKA keys")
}
func (f *sipFlow) InstallSecurityPlanRequest(ctx context.Context, r vc.IMSSecurityAssociationInstallRequest) error {
	if f.network == nil {
		return errors.New("IMS network unavailable")
	}
	s, e := newIMSSecurity(f.network.ip, f.remote.IP, f.localAgreement, r.Agreement, r.AKA)
	if e != nil {
		return e
	}
	f.network.security.Store(s)
	f.security = s
	return nil
}
func (f *sipFlow) UseSecurityAssociation(ctx context.Context, r vc.IMSSecurityAssociationInstallRequest) error {
	if f.security == nil {
		return errors.New("IMS security not installed")
	}
	if f.transport == "tcp" {
		remote := &net.UDPAddr{IP: f.remote.IP, Port: r.Agreement.PortServer}
		client, err := f.network.dialTCP(ctx, uint16(f.localAgreement.PortClient), remote)
		if err != nil {
			return err
		}
		f.mu.Lock()
		f.conn = client
		f.remote = remote
		f.sockets = append(f.sockets, client)
		f.mu.Unlock()
		go f.receive(client)
		return nil
	}
	client, e := f.network.listenUDP(uint16(f.localAgreement.PortClient))
	if e != nil {
		return e
	}
	server, e := f.network.listenUDP(uint16(f.localAgreement.PortServer))
	if e != nil {
		client.Close()
		return e
	}
	f.mu.Lock()
	f.conn = client
	f.remote = &net.UDPAddr{IP: f.remote.IP, Port: r.Agreement.PortServer}
	f.sockets = append(f.sockets, client, server)
	f.mu.Unlock()
	go f.receive(client)
	go f.receive(server)
	return nil
}
