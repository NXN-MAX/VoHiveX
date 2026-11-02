package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost/identity"
	vc "github.com/iniwex5/vowifi-go/runtimehost/voiceclient"
	vh "github.com/iniwex5/vowifi-go/runtimehost/voicehost"
	"github.com/wstgpt/swu-go/pkg/logger"
	"github.com/wstgpt/swu-go/pkg/swu"
	"go.uber.org/zap"
)

type engineState struct {
	DeviceID   string       `json:"device_id"`
	Phase      string       `json:"phase"`
	Registered bool         `json:"registered"`
	Tunnel     bool         `json:"tunnel"`
	Error      string       `json:"error,omitempty"`
	Calls      []engineCall `json:"calls"`
	UpdatedAt  time.Time    `json:"updated_at"`
}
type engineCall struct {
	ID              string     `json:"id"`
	DeviceID        string     `json:"device_id"`
	Number          string     `json:"number"`
	Direction       string     `json:"direction"`
	Status          string     `json:"status"`
	SIPCode         int        `json:"sip_code"`
	Detail          string     `json:"detail,omitempty"`
	NetworkReason   string     `json:"network_reason,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	ConnectedAt     *time.Time `json:"connected_at,omitempty"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	Transport       string     `json:"transport"`
	ReceivedPackets uint64     `json:"received_packets"`
	SentPackets     uint64     `json:"sent_packets"`
}
type engine struct {
	mu             sync.Mutex
	core           *coreClient
	state          engineState
	flow           *sipFlow
	network        *innerNetwork
	agent          *vh.IMSOutboundAgent
	profile        vc.IMSProfile
	binding        vc.RegistrationBinding
	session        vc.RegisterSession
	reg            vc.RegisterResult
	calls          map[string]*engineCall
	media          map[string]*callMedia
	ctx            context.Context
	cancel         context.CancelFunc
	epdg, mcc, mnc string
	recordDir      string
	sipTransport   string
}

func (e *engine) stage(phase string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state.Phase = phase
	e.state.UpdatedAt = time.Now().UTC()
	if err != nil {
		e.state.Error = err.Error()
	}
	if phase == "registered" {
		e.state.Registered = true
		e.state.Error = ""
	}
	if phase == "error" || phase == "stopped" {
		e.state.Registered = false
	}
}
func (e *engine) snapshot() engineState {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.state
	s.Calls = make([]engineCall, 0, len(e.calls))
	for id, c := range e.calls {
		v := *c
		if m := e.media[id]; m != nil {
			v.ReceivedPackets = m.rx.Load()
			v.SentPackets = m.tx.Load()
		}
		s.Calls = append(s.Calls, v)
	}
	return s
}
func (e *engine) run(ctx context.Context) error {
	ctx, e.cancel = context.WithCancel(ctx)
	defer e.cancel()
	e.mu.Lock()
	e.ctx = ctx
	e.calls = map[string]*engineCall{}
	e.media = map[string]*callMedia{}
	e.state.DeviceID = e.core.device
	e.mu.Unlock()
	defer func() { e.mu.Lock(); e.state.Tunnel = false; e.state.Registered = false; e.mu.Unlock() }()
	e.stage("checking", nil)
	if err := e.core.checkOwnership(ctx); err != nil {
		return err
	}
	imsi, err := e.core.GetIMSI()
	if err != nil {
		return fmt.Errorf("read SIM identity: %w", err)
	}
	if len(imsi) < 6 {
		return errors.New("invalid SIM identity")
	}
	mcc, mnc := e.mcc, e.mnc
	if mcc == "" {
		mcc = imsi[:3]
	}
	if mnc == "" {
		return errors.New("home MNC is required; do not infer MNC length from IMSI")
	}
	for len(mnc) < 3 {
		mnc = "0" + mnc
	}
	domain := "ims.mnc" + mnc + ".mcc" + mcc + ".3gppnetwork.org"
	profile := vc.IMSProfile{IMPI: imsi + "@" + domain, IMPU: "sip:" + imsi + "@" + domain, Domain: domain, UserAgent: "VoHiveX-IMS/0.1", AccessNetworkInfo: "IEEE-802.11"}
	e.core.mu.Lock()
	id, idErr := identity.ReadISIMIdentity(e.core.adapter)
	imei, _ := e.core.adapter.ReadIMEI()
	e.core.mu.Unlock()
	if idErr == nil {
		if id.IMPI != "" {
			profile.IMPI = id.IMPI
		}
		if id.Domain != "" {
			profile.Domain = id.Domain
		}
		if len(id.IMPU) > 0 {
			profile.IMPU = id.IMPU[0]
		}
	}
	epdg := e.epdg
	if epdg == "" {
		epdg = "epdg.epc.mnc" + mnc + ".mcc" + mcc + ".pub.3gppnetwork.org"
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", epdg)
	if err != nil || len(ips) == 0 {
		return errors.New("ePDG DNS lookup failed")
	}
	e.stage("authenticating", nil)
	// Silence upstream protocol logs: they can contain AKA identifiers/key material.
	logger.SetLogger(zap.NewNop())
	ready := make(chan struct{})
	var readyOnce sync.Once
	tunnel := swu.NewSession(&swu.Config{EpDGAddr: ips[0].String(), EpDGPort: 500, APN: "ims", SIM: swuSIM{e.core}, EnableDriver: true, DataplaneMode: "netstack", MCC: mcc, MNC: mnc, LocalPort: 0, OnReady: func() { readyOnce.Do(func() { close(ready) }) }}, nil)
	done := make(chan error, 1)
	go func() { done <- tunnel.Connect(ctx) }()
	select {
	case <-ready:
	case err := <-done:
		if err == nil {
			err = errors.New("tunnel closed before ready")
		}
		return fmt.Errorf("ePDG: %w", err)
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(90 * time.Second):
		e.cancel()
		return errors.New("ePDG authentication timeout")
	}
	snap := tunnel.Snapshot()
	if !snap.Established {
		return errors.New("no established ePDG tunnel")
	}
	ip := snap.IPv4
	pcscfs := snap.PCSCFv4
	if ip == nil {
		ip = snap.IPv6
		pcscfs = snap.PCSCFv6
	}
	if ip == nil || len(pcscfs) == 0 {
		return errors.New("ePDG did not provide inner IP and P-CSCF")
	}
	e.mu.Lock()
	e.state.Tunnel = true
	e.mu.Unlock()
	e.stage("registering", nil)
	n, err := newInnerNetwork(ctx, ip, tunnel)
	if err != nil {
		return err
	}
	defer n.close()
	e.network = n
	f, err := newSIPFlow(ctx, n, &net.UDPAddr{IP: pcscfs[0], Port: 5060}, e.sipTransport)
	if err != nil {
		return err
	}
	defer f.close()
	e.flow = f
	profile.LocalIP = ip.String()
	e.profile = profile
	agreement := vc.DefaultSecurityClientAgreement(nil)
	agreement.SPIClient = agreement.SPIClient&0x7fffffff | 0x100
	agreement.SPIServer = agreement.SPIServer&0x7fffffff | 0x100
	agreement.Parameters = map[string]string{"prot": "esp", "mod": "trans"}
	agreement.EncryptionAlgorithm = "aes-cbc"
	offers := []vc.SecurityAgreement{agreement}
	for _, v := range []struct{ alg, enc string }{{"hmac-sha-1-96", "null"}, {"hmac-md5-96", "aes-cbc"}, {"hmac-md5-96", "null"}} {
		a := agreement
		a.Algorithm = v.alg
		a.EncryptionAlgorithm = v.enc
		offers = append(offers, a)
	}
	f.localAgreement = agreement
	f.privateID, f.realm = profile.IMPI, profile.Domain
	if regexp.MustCompile(`^[0-9]{15}$`).MatchString(imei) {
		f.instance = "urn:gsma:imei:" + imei[:8] + "-" + imei[8:14] + "-" + imei[14:]
	}
	contact := "sip:" + imsi + "@" + net.JoinHostPort(ip.String(), fmt.Sprint(agreement.PortServer)) + ";transport=" + e.sipTransport
	session := vc.RegisterSession{Transport: f, AKAProvider: e.core, Profile: profile, RegistrarURI: "sip:" + profile.Domain, ContactURI: contact, CallID: randomID(), Expires: 3600, SecurityClient: agreement, SecurityClients: offers, SecurityPlanInstaller: f, SecurityLocalAddr: ip.String(), SecurityRemoteAddr: pcscfs[0].String()}
	regCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	reg, err := session.Register(regCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("IMS REGISTER: %w (%s)", err, f.registrationFailure)
	}
	if !reg.Registered {
		return fmt.Errorf("IMS REGISTER rejected: %d", reg.StatusCode)
	}
	e.mu.Lock()
	e.session = session
	e.reg = reg
	e.binding = reg.Binding
	e.agent = &vh.IMSOutboundAgent{Transport: f, Profile: profile, Registration: reg.Binding, UserAgent: profile.UserAgent, LocalTag: randomID(), SessionExpires: 1800}
	f.mu.Lock()
	f.onRequest = e.incoming
	f.onProvisional = e.provisional
	f.mu.Unlock()
	e.mu.Unlock()
	e.stage("registered", nil)
	refresh := time.NewTimer(time.Duration(max(30, reg.Binding.Expires/2)) * time.Second)
	defer refresh.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			if err == nil {
				err = errors.New("ePDG tunnel closed")
			}
			return err
		case <-refresh.C:
			rctx, c := context.WithTimeout(ctx, 35*time.Second)
			r, err := session.Refresh(rctx, vc.RefreshRequest{Binding: reg.Binding, CallID: session.CallID, CSeq: reg.NextCSeq, Expires: 3600, AuthHeader: reg.AuthHeader, AuthHeaderName: reg.AuthHeaderName, AuthState: reg.AuthState})
			c()
			if err != nil || !r.Refreshed {
				return errors.New("IMS registration refresh failed")
			}
			reg.Binding = r.Binding
			reg.NextCSeq = r.NextCSeq
			reg.AuthState = r.AuthState
			reg.AuthHeader = r.AuthHeader
			reg.AuthHeaderName = r.AuthHeaderName
			e.mu.Lock()
			e.binding = r.Binding
			e.agent.UpdateIMSRegistration(vh.IMSRegistrationUpdate{Registration: r.Binding})
			e.mu.Unlock()
			refresh.Reset(time.Duration(max(30, r.Binding.Expires/2)) * time.Second)
		}
	}
}

var e164 = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

func (e *engine) dial(number string) (engineCall, error) {
	if !e164.MatchString(number) {
		return engineCall{}, errors.New("number must use E.164 format")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.state.Registered || e.agent == nil {
		return engineCall{}, errors.New("IMS is not registered")
	}
	for _, c := range e.calls {
		if c.EndedAt == nil {
			return engineCall{}, errors.New("another call is active")
		}
	}
	id := randomID()
	m, err := newCallMedia(e.ctx, e.network, id, e.recordDir)
	if err != nil {
		return engineCall{}, err
	}
	c := &engineCall{ID: id, DeviceID: e.core.device, Number: number, Direction: "outgoing", Status: "dialing", StartedAt: time.Now().UTC(), Transport: "vowifi"}
	e.calls[id] = c
	e.media[id] = m
	go func() {
		// A global telephone number must be marked as a telephone URI. A bare
		// SIP user can be interpreted as an IMS account instead of a PSTN number.
		result, err := e.agent.StartOutboundCall(e.ctx, vh.OutboundCallRequest{CallID: id, DeviceID: e.core.device, Callee: "tel:" + number, RawSDP: m.offer()})
		e.mu.Lock()
		defer e.mu.Unlock()
		c.SIPCode = result.StatusCode
		c.NetworkReason = stringHeader(result.Headers, "Reason")
		if err != nil {
			c.Status = "uncertain"
			c.Detail = "No conclusive SIP result; not retried automatically"
			return
		}
		if !result.Accepted {
			c.Status = "failed"
			c.Detail = result.Reason
			now := time.Now().UTC()
			c.EndedAt = &now
			m.close()
			return
		}
		if err = m.negotiate(result.RawSDP); err != nil {
			c.Status = "active"
			c.Detail = err.Error()
		} else {
			c.Status = "active"
		}
		now := time.Now().UTC()
		c.ConnectedAt = &now
	}()
	return *c, nil
}
func (e *engine) provisional(req vc.SIPRequestMessage, res vc.SIPResponse) {
	if req.Method != "INVITE" {
		return
	}
	id := stringHeader(req.Headers, "Call-ID")
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.calls[id]
	if c == nil {
		return
	}
	c.SIPCode = res.StatusCode
	if res.StatusCode == 180 {
		c.Status = "alerting"
		c.Detail = "Network reported ringing; handset reception is not yet confirmed"
	} else if res.StatusCode == 183 {
		c.Status = "early_media"
	}
	if len(res.Body) > 0 {
		if m := e.media[id]; m != nil {
			if err := m.negotiate(res.Body); err != nil {
				c.Detail = err.Error()
			}
		}
	}
}
func (e *engine) incoming(req vc.SIPIncomingRequest, reply func(int, string, map[string]string, []byte)) {
	switch strings.ToUpper(req.Method) {
	case "OPTIONS":
		reply(200, "OK", map[string]string{"Allow": "INVITE, ACK, CANCEL, BYE, PRACK, UPDATE, INFO, MESSAGE, OPTIONS"}, nil)
	case "BYE":
		id := header(req.Headers, "Call-ID")
		e.mu.Lock()
		c := e.calls[id]
		if c == nil {
			e.mu.Unlock()
			reply(481, "Call Does Not Exist", nil, nil)
			return
		}
		now := time.Now().UTC()
		c.EndedAt = &now
		c.Status = "ended"
		if m := e.media[id]; m != nil {
			m.close()
		}
		e.mu.Unlock()
		reply(200, "OK", nil, nil)
	case "ACK":
		return
	default:
		reply(501, "Not Implemented", nil, nil)
	}
}
func (e *engine) hangup(id string) error {
	e.mu.Lock()
	c := e.calls[id]
	agent := e.agent
	if c == nil || c.EndedAt != nil {
		e.mu.Unlock()
		return errors.New("call is not active")
	}
	e.mu.Unlock()
	ctx, cancel := context.WithTimeout(e.ctx, 35*time.Second)
	defer cancel()
	err := agent.EndVoiceCall(ctx, vh.DialogInfo{CallID: id})
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now().UTC()
	c.EndedAt = &now
	c.Status = "ended"
	if m := e.media[id]; m != nil {
		m.close()
	}
	return nil
}
