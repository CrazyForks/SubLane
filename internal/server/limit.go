package server

import (
	"math"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const maxLoginPeers = 1024

type attemptWindow struct {
	count    int
	reset    time.Time
	lastSeen time.Time
}

type LoginLimiter struct {
	mu    sync.Mutex
	now   func() time.Time
	peers map[string]attemptWindow
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{now: time.Now, peers: make(map[string]attemptWindow)}
}

func newLoginLimiter() *LoginLimiter { return NewLoginLimiter() }

func (l *LoginLimiter) allow(peer string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	retry := func(reset time.Time) int { return max(1, int(math.Ceil(reset.Sub(now).Seconds()))) }
	for key, window := range l.peers {
		if !now.Before(window.reset) {
			delete(l.peers, key)
		}
	}
	window, exists := l.peers[peer]
	if !exists {
		if len(l.peers) >= maxLoginPeers {
			oldest := ""
			var oldestSeen time.Time
			found := false
			for key, entry := range l.peers {
				if !found || entry.lastSeen.Before(oldestSeen) {
					oldest, oldestSeen, found = key, entry.lastSeen, true
				}
			}
			// A full tracking map must not become a site-wide login denial for new peers.
			delete(l.peers, oldest)
		}
		window = attemptWindow{reset: now.Add(time.Minute)}
	}
	window.lastSeen = now
	if window.count >= 5 {
		l.peers[peer] = window
		return retry(window.reset)
	}
	window.count++
	l.peers[peer] = window
	return 0
}

func peerAddress(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return "unknown"
	}
	ip = ip.Unmap()
	if !isTrustedProxy(ip, trusted) {
		return ip.String()
	}
	forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if forwarded == "" {
		return ip.String()
	}
	chain := strings.Split(forwarded, ",")
	for i := len(chain) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil {
			return ip.String()
		}
		hop = hop.Unmap()
		if !isTrustedProxy(hop, trusted) || i == 0 {
			return hop.String()
		}
	}
	return ip.String()
}

func isTrustedProxy(ip netip.Addr, trusted []netip.Prefix) bool {
	for _, prefix := range trusted {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
