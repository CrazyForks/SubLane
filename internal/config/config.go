package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr            string
	DataDir         string
	LogLevel        slog.Level
	PublicURL       string
	TrustedProxies  []netip.Prefix
	PricingURL      string
	PricingHashURL  string
	PricingCache    string
	PricingFallback string
	PricingOverride string
	PricingInterval time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{Addr: "127.0.0.1:8080", DataDir: "./data", LogLevel: slog.LevelInfo, PricingInterval: 6 * time.Hour}
	if v := getenv("SUBLANE_ADDR"); v != "" {
		c.Addr = v
	}
	if v := getenv("SUBLANE_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := getenv("SUBLANE_LOG_LEVEL"); v != "" {
		if err := c.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return c, fmt.Errorf("SUBLANE_LOG_LEVEL: %w", err)
		}
	}
	if v := getenv("SUBLANE_PUBLIC_URL"); v != "" {
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return c, fmt.Errorf("SUBLANE_PUBLIC_URL must be an http(s) origin without credentials, path, query, or fragment")
		}
		host := strings.ToLower(u.Host)
		if port := u.Port(); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return c, fmt.Errorf("invalid SUBLANE_PUBLIC_URL port")
			}
			if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
				host = strings.ToLower(u.Hostname())
				if strings.Contains(host, ":") {
					host = "[" + host + "]"
				}
			}
		}
		c.PublicURL = u.Scheme + "://" + host
	}
	if v := getenv("SUBLANE_TRUSTED_PROXIES"); v != "" {
		for _, part := range strings.Split(v, ",") {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
			if err != nil {
				return c, fmt.Errorf("SUBLANE_TRUSTED_PROXIES must be comma-separated IP CIDRs: %w", err)
			}
			c.TrustedProxies = append(c.TrustedProxies, prefix.Masked())
		}
	}
	c.PricingURL = getenv("SUBLANE_PRICING_URL")
	c.PricingHashURL = getenv("SUBLANE_PRICING_HASH_URL")
	c.PricingCache = getenv("SUBLANE_PRICING_CACHE")
	c.PricingFallback = getenv("SUBLANE_PRICING_FALLBACK")
	c.PricingOverride = getenv("SUBLANE_PRICING_OVERRIDE")
	if v := getenv("SUBLANE_PRICING_INTERVAL"); v != "" {
		interval, err := time.ParseDuration(v)
		if err != nil || interval <= 0 {
			return c, fmt.Errorf("SUBLANE_PRICING_INTERVAL must be a positive duration")
		}
		c.PricingInterval = interval
	}
	_, port, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return c, fmt.Errorf("SUBLANE_ADDR must be host:port: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return c, fmt.Errorf("SUBLANE_ADDR port must be between 1 and 65535")
	}
	return c, nil
}
