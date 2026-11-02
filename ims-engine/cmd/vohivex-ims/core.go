package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	sim "github.com/iniwex5/vowifi-go/engine/sim"
	"github.com/iniwex5/vowifi-go/runtimehost/simauth"
	"github.com/iniwex5/vowifi-go/runtimehost/simtransport"
	externalsim "github.com/wstgpt/swu-go/pkg/sim"
	"gopkg.in/yaml.v3"
)

type localAT interface {
	ExecuteATSilent(string, time.Duration) (string, error)
	Close() error
}

type coreClient struct {
	local               localAT
	base, device, token string
	client              *http.Client
	mu                  sync.Mutex
	adapter             *simtransport.Adapter
	aka                 *simauth.AKAProvider
	imsi                string
}

func newCore(base, device, config string) (*coreClient, error) {
	u, e := url.Parse(base)
	if e != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		return nil, errors.New("core endpoint must use container loopback")
	}
	b, e := os.ReadFile(config)
	if e != nil {
		return nil, e
	}
	var cfg struct {
		Web struct{ Username, Password string }
	}
	if e = yaml.Unmarshal(b, &cfg); e != nil {
		return nil, e
	}
	c := &coreClient{base: strings.TrimRight(base, "/"), device: device, client: &http.Client{Timeout: 20 * time.Second}}
	var login struct {
		Token string `json:"token"`
	}
	if e = c.request(context.Background(), "POST", "/api/auth/login", map[string]string{"username": cfg.Web.Username, "password": cfg.Web.Password}, &login); e != nil {
		return nil, e
	}
	if login.Token == "" {
		return nil, errors.New("core authentication returned no token")
	}
	c.token = login.Token
	c.adapter = simtransport.NewAdapter(c)
	c.aka = simauth.NewAKAProvider(c.adapter)
	return c, nil
}
func (c *coreClient) request(ctx context.Context, method, path string, body, out any) error {
	var data io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		data = bytes.NewReader(b)
	}
	r, e := http.NewRequestWithContext(ctx, method, c.base+path, data)
	if e != nil {
		return e
	}
	r.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, e := c.client.Do(r)
	if e != nil {
		return errors.New("core connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("core request %s returned HTTP %d", path, resp.StatusCode)
	}
	if out == nil {
		_, e = io.Copy(io.Discard, resp.Body)
		return e
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
}
func (c *coreClient) ExecuteATSilent(command string, timeout time.Duration) (string, error) {
	if c.local != nil {
		return c.local.ExecuteATSilent(command, timeout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+3*time.Second)
	defer cancel()
	var value map[string]any
	if e := c.request(ctx, "POST", "/api/devices/"+url.PathEscape(c.device)+"/actions/at", map[string]any{"cmd": command, "timeout_ms": timeout.Milliseconds()}, &value); e != nil {
		return "", e
	}
	for _, k := range []string{"response", "result", "data", "output"} {
		if s, ok := value[k].(string); ok {
			return s, nil
		}
	}
	return "", errors.New("core AT response missing")
}
func (c *coreClient) GetIMSI() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.imsi != "" {
		return c.imsi, nil
	}
	v, e := c.adapter.ReadIMSI()
	if e == nil {
		c.imsi = v
	}
	return v, e
}
func (c *coreClient) CalculateAKA(rand, autn []byte) (sim.AKAResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.aka.CalculateAKA(rand, autn)
}
func (c *coreClient) Close() error {
	if c.local != nil {
		return c.local.Close()
	}
	return nil
}

type swuSIM struct{ c *coreClient }

func (a swuSIM) GetIMSI() (string, error) { return a.c.GetIMSI() }
func (a swuSIM) CalculateAKA(rand, autn []byte) (res, ck, ik, auts []byte, err error) {
	v, e := a.c.CalculateAKA(rand, autn)
	if errors.Is(e, sim.ErrSyncFailure) {
		e = externalsim.ErrSyncFailure
	}
	return v.RES, v.CK, v.IK, v.AUTS, e
}
func (a swuSIM) Close() error { return nil }

// Do not compete with the existing IMS registration or enable a cellular PDP.
func (c *coreClient) checkOwnership(ctx context.Context) error {
	if c.local != nil {
		for _, q := range []struct{ cmd, required string }{{"AT+CFUN?", "+CFUN: 4"}, {"AT+CGATT?", "+CGATT: 0"}} {
			v, err := c.ExecuteATSilent(q.cmd, 5*time.Second)
			if err != nil {
				return err
			}
			if !regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(q.required) + `\s*$`).MatchString(v) {
				return errors.New("local USB debug requires RF off (CFUN=4) and no cellular attachment")
			}
		}
		pdp, err := c.ExecuteATSilent("AT+CGACT?", 5*time.Second)
		if err != nil {
			return err
		}
		if !regexp.MustCompile(`(?m)^\s*OK\s*$`).MatchString(pdp) || regexp.MustCompile(`(?m)^\s*\+CGACT:\s*\d+\s*,\s*1\s*$`).MatchString(pdp) {
			return errors.New("cannot confirm inactive cellular PDP contexts")
		}
		return nil
	}

	var list struct {
		Devices []map[string]any `json:"devices"`
	}
	if e := c.request(ctx, "GET", "/api/devices", nil, &list); e != nil {
		return e
	}
	for _, d := range list.Devices {
		if d["id"] == c.device {
			if d["vowifi_enabled"] != false || d["vowifi_active"] == true {
				return errors.New("original VoWiFi runtime must be stopped before starting IMS engine")
			}
			if d["network_enabled"] != false {
				return errors.New("cellular data must stay disabled")
			}
			return nil
		}
	}
	return errors.New("device not found")
}
