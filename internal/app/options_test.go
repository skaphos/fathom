/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func newTestFlags(t *testing.T) (*pflag.FlagSet, *zap.Options) {
	t.Helper()
	fs := pflag.NewFlagSet("fathom-test", pflag.ContinueOnError)
	var zapOpts zap.Options
	RegisterFlags(fs, &zapOpts)
	return fs, &zapOpts
}

func TestDefaultOptions_MatchFlagDefaults(t *testing.T) {
	fs, zapOpts := newTestFlags(t)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse empty args: %v", err)
	}

	got, err := Load(fs, *zapOpts, "", false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := DefaultOptions()
	if got.Metrics != want.Metrics {
		t.Errorf("Metrics: got %+v, want %+v", got.Metrics, want.Metrics)
	}
	if got.Webhook != want.Webhook {
		t.Errorf("Webhook: got %+v, want %+v", got.Webhook, want.Webhook)
	}
	if got.HealthProbeBindAddress != want.HealthProbeBindAddress {
		t.Errorf("HealthProbeBindAddress: got %q, want %q", got.HealthProbeBindAddress, want.HealthProbeBindAddress)
	}
	if got.LeaderElect != want.LeaderElect {
		t.Errorf("LeaderElect: got %v, want %v", got.LeaderElect, want.LeaderElect)
	}
	if got.EnableHTTP2 != want.EnableHTTP2 {
		t.Errorf("EnableHTTP2: got %v, want %v", got.EnableHTTP2, want.EnableHTTP2)
	}
}

func TestLoad_FlagOverridesEverything(t *testing.T) {
	t.Setenv("FATHOM_METRICS_BIND_ADDRESS", ":1111")

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "fathom.yaml")
	if err := os.WriteFile(configPath, []byte(`
metrics:
  bind_address: ":2222"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fs, zapOpts := newTestFlags(t)
	if err := fs.Parse([]string{"--metrics-bind-address=:3333"}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	got, err := Load(fs, *zapOpts, configPath, true)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Metrics.BindAddress != ":3333" {
		t.Errorf("flag should win: got %q, want :3333", got.Metrics.BindAddress)
	}
}

func TestLoad_EnvOverridesConfig(t *testing.T) {
	t.Setenv("FATHOM_METRICS_BIND_ADDRESS", ":1111")

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "fathom.yaml")
	if err := os.WriteFile(configPath, []byte(`
metrics:
  bind_address: ":2222"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fs, zapOpts := newTestFlags(t)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}

	got, err := Load(fs, *zapOpts, configPath, true)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Metrics.BindAddress != ":1111" {
		t.Errorf("env should beat config: got %q, want :1111", got.Metrics.BindAddress)
	}
}

func TestLoad_ConfigOverridesDefault(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "fathom.yaml")
	if err := os.WriteFile(configPath, []byte(`
metrics:
  bind_address: ":2222"
  secure: false
webhook:
  cert_path: /etc/webhook
leader_elect: true
leader_election_id: custom.example.com
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fs, zapOpts := newTestFlags(t)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}

	got, err := Load(fs, *zapOpts, configPath, true)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Metrics.BindAddress != ":2222" {
		t.Errorf("Metrics.BindAddress: got %q, want :2222", got.Metrics.BindAddress)
	}
	if got.Metrics.Secure {
		t.Errorf("Metrics.Secure: got true, want false")
	}
	if got.Webhook.CertPath != "/etc/webhook" {
		t.Errorf("Webhook.CertPath: got %q, want /etc/webhook", got.Webhook.CertPath)
	}
	if !got.LeaderElect {
		t.Errorf("LeaderElect: got false, want true")
	}
	if got.LeaderElectionID != "custom.example.com" {
		t.Errorf("LeaderElectionID: got %q, want custom.example.com", got.LeaderElectionID)
	}
}

func TestLoad_ProbeImagePrecedence(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "fathom.yaml")
	if err := os.WriteFile(configPath, []byte(`
probe_image: registry.example.com/probe:from-config
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Run("default when nothing set", func(t *testing.T) {
		fs, zapOpts := newTestFlags(t)
		if err := fs.Parse(nil); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got, err := Load(fs, *zapOpts, "", false)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.ProbeImage != DefaultProbeImage {
			t.Errorf("ProbeImage: got %q, want default %q", got.ProbeImage, DefaultProbeImage)
		}
	})

	t.Run("config beats default", func(t *testing.T) {
		fs, zapOpts := newTestFlags(t)
		if err := fs.Parse(nil); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got, err := Load(fs, *zapOpts, configPath, true)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.ProbeImage != "registry.example.com/probe:from-config" {
			t.Errorf("ProbeImage: got %q", got.ProbeImage)
		}
	})

	t.Run("env beats config", func(t *testing.T) {
		t.Setenv("FATHOM_PROBE_IMAGE", "registry.example.com/probe:from-env")
		fs, zapOpts := newTestFlags(t)
		if err := fs.Parse(nil); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got, err := Load(fs, *zapOpts, configPath, true)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.ProbeImage != "registry.example.com/probe:from-env" {
			t.Errorf("ProbeImage: got %q", got.ProbeImage)
		}
	})

	t.Run("flag beats env", func(t *testing.T) {
		t.Setenv("FATHOM_PROBE_IMAGE", "registry.example.com/probe:from-env")
		fs, zapOpts := newTestFlags(t)
		if err := fs.Parse([]string{"--probe-image=registry.example.com/probe:from-flag"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got, err := Load(fs, *zapOpts, configPath, true)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.ProbeImage != "registry.example.com/probe:from-flag" {
			t.Errorf("ProbeImage: got %q", got.ProbeImage)
		}
	})
}

func TestLoad_MissingDefaultConfig_Ignored(t *testing.T) {
	fs, zapOpts := newTestFlags(t)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	if _, err := Load(fs, *zapOpts, missing, false); err != nil {
		t.Errorf("missing default-path config should be ignored, got: %v", err)
	}
}

func TestLoad_MissingExplicitConfig_Errors(t *testing.T) {
	fs, zapOpts := newTestFlags(t)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	_, err := Load(fs, *zapOpts, missing, true)
	if err == nil {
		t.Fatal("expected error for missing explicit config, got nil")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Errorf("error wrap: got %q, want contains 'read config'", err.Error())
	}
}

func TestLoad_MalformedConfig_Errors(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "fathom.yaml")
	if err := os.WriteFile(configPath, []byte("this is: not: valid: yaml: ["), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fs, zapOpts := newTestFlags(t)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := Load(fs, *zapOpts, configPath, true); err == nil {
		t.Fatal("expected error for malformed config")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{
			name:   "defaults are valid",
			mutate: func(*Options) {},
		},
		{
			name: "webhook cert path without name",
			mutate: func(o *Options) {
				o.Webhook.CertPath = "/etc/webhook"
				o.Webhook.CertName = ""
			},
			wantErr: "webhook.cert_name",
		},
		{
			name: "webhook cert path without key",
			mutate: func(o *Options) {
				o.Webhook.CertPath = "/etc/webhook"
				o.Webhook.CertKey = ""
			},
			wantErr: "webhook.cert_key",
		},
		{
			name: "metrics cert path without name",
			mutate: func(o *Options) {
				o.Metrics.CertPath = "/etc/metrics"
				o.Metrics.CertName = ""
			},
			wantErr: "metrics.cert_name",
		},
		{
			name: "empty health probe address",
			mutate: func(o *Options) {
				o.HealthProbeBindAddress = ""
			},
			wantErr: "health_probe_bind_address",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultOptions()
			tc.mutate(&opts)
			err := opts.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
