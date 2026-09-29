package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var ErrInput = errors.New("invalid_alert_settings")
var ErrDelivery = errors.New("alert_delivery_failed")

type Sender interface {
	Do(*http.Request) (*http.Response, error)
}

func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16"} {
		if netip.MustParsePrefix(prefix).Contains(ip) {
			return false
		}
	}
	return true
}

func destination(raw string) (*url.URL, error) {
	if len(raw) > 2048 || strings.TrimSpace(raw) != raw {
		return nil, ErrInput
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, ErrInput
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if len(host) > 253 {
		return nil, ErrInput
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, ErrInput
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !publicIP(ip) {
			return nil, ErrInput
		}
	} else if !strings.Contains(host, ".") {
		return nil, ErrInput
	}
	if port := u.Port(); port != "" {
		if _, err := net.LookupPort("tcp", port); err != nil {
			return nil, ErrInput
		}
	}
	return u, nil
}

func newSender() *http.Client {
	transport := &http.Transport{
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 16 << 10,
		IdleConnTimeout: 30 * time.Second, MaxIdleConns: 4, MaxConnsPerHost: 2,
		// Resolve and pin a public address at dial time; redirects and environment proxies cannot bypass this check.
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, ErrDelivery
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return nil, ErrDelivery
			}
			for _, ip := range ips {
				if !publicIP(ip) {
					return nil, ErrDelivery
				}
			}
			dialer := net.Dialer{Timeout: 5 * time.Second}
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
			}
			return nil, ErrDelivery
		},
	}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrDelivery }}
}

func deliver(ctx context.Context, sender Sender, endpoint string, event Event) error {
	if _, err := destination(endpoint); err != nil {
		return ErrDelivery
	}
	body, err := json.Marshal(event)
	if err != nil {
		return ErrDelivery
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrDelivery
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "SubLane-Webhook/1")
	request.Header.Set("Idempotency-Key", event.ID)
	response, err := sender.Do(request)
	if err != nil {
		return ErrDelivery
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ErrDelivery
	}
	return nil
}
