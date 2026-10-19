package config

import "testing"

func lookup(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := env[k]; return v, ok }
}

func TestDefaults(t *testing.T) {
	cfg, err := FromLookup(lookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 || cfg.DataDir != "./data" || cfg.AuthMode != AuthPassword || cfg.BaseURL != "" ||
		cfg.AuthModeSet || cfg.DataDirSet || cfg.ProxyAuthHeader != "Remote-User" || len(cfg.TrustedProxies) != 0 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestValidValues(t *testing.T) {
	cfg, err := FromLookup(lookup(map[string]string{
		"PORT": "9000", "HOST": "127.0.0.1", "DATA_DIR": "/data", "BASE_URL": "https://stats.example.com/", "AUTH_MODE": "Proxy",
		"TRUSTED_PROXIES": "172.16.0.0/12, 10.0.0.2", "AUTH_PROXY_HEADER": "X-Forwarded-User",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9000 || cfg.Host != "127.0.0.1" || cfg.DataDir != "/data" || cfg.BaseURL != "https://stats.example.com" || cfg.AuthMode != AuthProxy ||
		!cfg.AuthModeSet || !cfg.DataDirSet || len(cfg.TrustedProxies) != 2 || cfg.ProxyAuthHeader != "X-Forwarded-User" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestInvalidValues(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"port text":    {"PORT": "abc"},
		"port range":   {"PORT": "70000"},
		"base scheme":  {"BASE_URL": "ftp://x"},
		"base no host": {"BASE_URL": "https://"},
		"base query":   {"BASE_URL": "https://x/?a=1"},
		"auth unknown": {"AUTH_MODE": "oidc"},
		"host url":     {"HOST": "http://x"},
		"proxy no ips": {"AUTH_MODE": "proxy"},
		"bad proxies":  {"TRUSTED_PROXIES": "10.0.0.0/99"},
		"bad header":   {"AUTH_PROXY_HEADER": "Remote User"},
	} {
		if _, err := FromLookup(lookup(env)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
