package config

import (
	"net/netip"
	"testing"
)

func TestTrustedProxyCIDRs(t *testing.T) {
	cfg, err := Load(func(k string) string {
		if k == "SUBLANE_TRUSTED_PROXIES" {
			return "127.0.0.0/8, ::1/128"
		}
		return ""
	})
	if err != nil || len(cfg.TrustedProxies) != 2 || !cfg.TrustedProxies[0].Contains(netip.MustParseAddr("127.0.0.1")) {
		t.Fatalf("trusted proxies: %+v %v", cfg.TrustedProxies, err)
	}
	if _, err := Load(func(k string) string {
		if k == "SUBLANE_TRUSTED_PROXIES" {
			return "0.0.0.0/0,invalid"
		}
		return ""
	}); err == nil {
		t.Fatal("invalid trusted proxy was accepted")
	}
}

func TestDefaultsAndOverrides(t *testing.T) {
	cfg, err := Load(func(string) string { return "" })
	if err != nil || cfg.Addr != "127.0.0.1:8080" || cfg.DataDir != "./data" || cfg.LogLevel.String() != "INFO" {
		t.Fatalf("unexpected defaults: %+v, %v", cfg, err)
	}
	env := map[string]string{"SUBLANE_ADDR": "[::1]:9000", "SUBLANE_DATA_DIR": "/tmp/synthetic-data", "SUBLANE_LOG_LEVEL": "debug"}
	cfg, err = Load(func(k string) string { return env[k] })
	if err != nil || cfg.Addr != "[::1]:9000" || cfg.LogLevel.String() != "DEBUG" || cfg.DataDir != env["SUBLANE_DATA_DIR"] {
		t.Fatalf("overrides not applied: %+v, %v", cfg, err)
	}
}

func TestRejectsInvalidConfig(t *testing.T) {
	for _, env := range []map[string]string{
		{"SUBLANE_ADDR": "localhost"}, {"SUBLANE_ADDR": "127.0.0.1:0"},
		{"SUBLANE_ADDR": "127.0.0.1:70000"}, {"SUBLANE_LOG_LEVEL": "quiet"},
	} {
		if _, err := Load(func(k string) string { return env[k] }); err == nil {
			t.Fatalf("accepted invalid config: %v", env)
		}
	}
}

func TestPublicOrigin(t *testing.T) {
	for _, value := range []string{"ftp://example.test", "https://user@example.test", "https://example.test/path", "https://example.test?x=1", "https://example.test/#x", "not-a-url", "https://example.test:99999"} {
		if _, err := Load(func(k string) string {
			if k == "SUBLANE_PUBLIC_URL" {
				return value
			}
			return ""
		}); err == nil {
			t.Fatalf("invalid origin accepted: %s", value)
		}
	}
	cfg, err := Load(func(k string) string {
		if k == "SUBLANE_PUBLIC_URL" {
			return "https://GATEWAY.example.test:443/"
		}
		return ""
	})
	if err != nil || cfg.PublicURL != "https://gateway.example.test" {
		t.Fatalf("origin normalization: %+v %v", cfg, err)
	}
}
