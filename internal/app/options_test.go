/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
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
	// SKA-303: leader election defaults on. Assert the concrete value (not just
	// got==want) so this fails if the in-code default ever regresses to false.
	if !want.LeaderElect {
		t.Errorf("DefaultOptions().LeaderElect: got false, want true")
	}
	if got.Metrics != want.Metrics {
		t.Errorf("Metrics: got %+v, want %+v", got.Metrics, want.Metrics)
	}
	if got.Webhook != want.Webhook {
		t.Errorf("Webhook: got %+v, want %+v", got.Webhook, want.Webhook)
	}
	if got.Tracing != want.Tracing {
		t.Errorf("Tracing: got %+v, want %+v", got.Tracing, want.Tracing)
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
leader_elect: false
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
	// Config sets leader_elect: false, which must override the true default.
	if got.LeaderElect {
		t.Errorf("LeaderElect: got true, want false (config should override the true default)")
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

func TestLoad_NodeAgentImagePrecedence(t *testing.T) {
	t.Run("default when nothing set", func(t *testing.T) {
		fs, zapOpts := newTestFlags(t)
		if err := fs.Parse(nil); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got, err := Load(fs, *zapOpts, "", false)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.NodeAgentImage != DefaultNodeAgentImage {
			t.Errorf("NodeAgentImage: got %q, want default %q", got.NodeAgentImage, DefaultNodeAgentImage)
		}
	})

	t.Run("env beats default", func(t *testing.T) {
		t.Setenv("FATHOM_NODE_AGENT_IMAGE", "registry.example.com/node-agent:from-env")
		fs, zapOpts := newTestFlags(t)
		if err := fs.Parse(nil); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got, err := Load(fs, *zapOpts, "", false)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.NodeAgentImage != "registry.example.com/node-agent:from-env" {
			t.Errorf("NodeAgentImage: got %q", got.NodeAgentImage)
		}
	})

	t.Run("flag beats env", func(t *testing.T) {
		t.Setenv("FATHOM_NODE_AGENT_IMAGE", "registry.example.com/node-agent:from-env")
		fs, zapOpts := newTestFlags(t)
		if err := fs.Parse([]string{"--node-agent-image=registry.example.com/node-agent:from-flag"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		got, err := Load(fs, *zapOpts, "", false)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.NodeAgentImage != "registry.example.com/node-agent:from-flag" {
			t.Errorf("NodeAgentImage: got %q", got.NodeAgentImage)
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
		{
			name: "insecure metrics on network port is rejected",
			mutate: func(o *Options) {
				o.Metrics.BindAddress = ":8080"
				o.Metrics.Secure = false
				o.Metrics.AllowInsecure = false
			},
			wantErr: "refusing to expose plaintext metrics",
		},
		{
			name: "insecure metrics opt-in is honored",
			mutate: func(o *Options) {
				o.Metrics.BindAddress = ":8080"
				o.Metrics.Secure = false
				o.Metrics.AllowInsecure = true
			},
		},
		{
			name: "insecure metrics with bind_address=0 stays valid",
			mutate: func(o *Options) {
				o.Metrics.BindAddress = "0"
				o.Metrics.Secure = false
				o.Metrics.AllowInsecure = false
			},
		},
		{
			name: "secure metrics on network port stays valid",
			mutate: func(o *Options) {
				o.Metrics.BindAddress = ":8443"
				o.Metrics.Secure = true
			},
		},
		{
			name: "malformed metrics bind address",
			mutate: func(o *Options) {
				o.Metrics.BindAddress = "not-a-host-port"
				o.Metrics.Secure = true
			},
			wantErr: "metrics.bind_address",
		},
		{
			name: "malformed health_probe bind address",
			mutate: func(o *Options) {
				o.HealthProbeBindAddress = "no-colon-here"
			},
			wantErr: "health_probe_bind_address",
		},
		{
			name: "health_probe bind address \"0\" disables and skips check",
			mutate: func(o *Options) {
				o.HealthProbeBindAddress = "0"
			},
		},
		{
			name: "malformed leader_election_id",
			mutate: func(o *Options) {
				o.LeaderElectionID = "NotADns_Subdomain!"
			},
			wantErr: "leader_election_id",
		},
		{
			name: "empty leader_election_id is skipped",
			mutate: func(o *Options) {
				o.LeaderElectionID = ""
			},
		},
		{
			name: "tracing sampling ratio above 1 is rejected",
			mutate: func(o *Options) {
				o.Tracing.SamplingRatio = 1.5
			},
			wantErr: "tracing.sampling_ratio",
		},
		{
			name: "tracing sampling ratio below 0 is rejected",
			mutate: func(o *Options) {
				o.Tracing.SamplingRatio = -0.1
			},
			wantErr: "tracing.sampling_ratio",
		},
		{
			name: "tracing sampling ratio at bounds is valid",
			mutate: func(o *Options) {
				o.Tracing.SamplingRatio = 0
			},
		},
		{
			name: "multiple errors are joined",
			mutate: func(o *Options) {
				o.HealthProbeBindAddress = ""
				o.LeaderElectionID = "BAD!"
			},
			wantErr: "leader_election_id",
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

// TestValidate_MultipleErrorsAccumulate confirms errors.Join still surfaces
// every distinct failure in a single Validate pass (preserved invariant from
// SKA-299) — important for ops teams diagnosing misconfigured ConfigMaps.
func TestValidate_MultipleErrorsAccumulate(t *testing.T) {
	opts := DefaultOptions()
	opts.HealthProbeBindAddress = ""
	opts.LeaderElectionID = "BAD!"
	opts.Metrics.BindAddress = "bogus"

	err := opts.Validate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, want := range []string{"health_probe_bind_address", "leader_election_id", "metrics.bind_address"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("joined error %q is missing %q", err.Error(), want)
		}
	}
}

// TestLoad_TracingFlags confirms the SKA-293 tracing flags (including the
// float64 --tracing-sampling-ratio) flow through the flag→viper→Options
// pipeline, exercising the float branch added to bindings()/RegisterFlags/Load.
func TestLoad_TracingFlags(t *testing.T) {
	fs, zapOpts := newTestFlags(t)
	if err := fs.Parse([]string{
		"--tracing-enabled=true",
		"--tracing-otlp-endpoint=collector.observability:4317",
		"--tracing-sampling-ratio=0.25",
		"--tracing-insecure=true",
	}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := Load(fs, *zapOpts, "", false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := TracingOptions{
		Enabled:       true,
		OTLPEndpoint:  "collector.observability:4317",
		SamplingRatio: 0.25,
		Insecure:      true,
	}
	if got.Tracing != want {
		t.Errorf("Tracing: got %+v, want %+v", got.Tracing, want)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("Validate with tracing flags: %v", err)
	}
}

// TestLoad_TracingEnvAndConfig confirms env vars (which arrive as strings)
// decode into the bool/float tracing fields via WeaklyTypedInput, and that env
// still beats config per the precedence rules.
func TestLoad_TracingEnvAndConfig(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "fathom.yaml")
	if err := os.WriteFile(configPath, []byte(`
tracing:
  enabled: false
  sampling_ratio: 0.1
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("FATHOM_TRACING_ENABLED", "true")
	t.Setenv("FATHOM_TRACING_SAMPLING_RATIO", "0.5")

	fs, zapOpts := newTestFlags(t)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := Load(fs, *zapOpts, configPath, true)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Tracing.Enabled {
		t.Error("Tracing.Enabled: env true should beat config false")
	}
	if got.Tracing.SamplingRatio != 0.5 {
		t.Errorf("Tracing.SamplingRatio: got %v, want 0.5 (env beats config)", got.Tracing.SamplingRatio)
	}
}

// TestLoad_TracingSamplingRatioFromConfig confirms a config-file float decodes
// when no env/flag overrides it (the config-beats-default precedence rung).
func TestLoad_TracingSamplingRatioFromConfig(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "fathom.yaml")
	if err := os.WriteFile(configPath, []byte(`
tracing:
  sampling_ratio: 0.75
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
	if got.Tracing.SamplingRatio != 0.75 {
		t.Errorf("Tracing.SamplingRatio: got %v, want 0.75", got.Tracing.SamplingRatio)
	}
}

// TestLoad_MetricsAllowInsecureFlag confirms the new --metrics-allow-insecure
// flag is wired through the flag→viper→Options pipeline. Together with the
// Validate insecure-metrics test cases this end-to-ends the SKA-287 opt-in.
func TestLoad_MetricsAllowInsecureFlag(t *testing.T) {
	fs, zapOpts := newTestFlags(t)
	if err := fs.Parse([]string{
		"--metrics-allow-insecure=true",
		"--metrics-bind-address=:8080",
		"--metrics-secure=false",
	}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := Load(fs, *zapOpts, "", false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Metrics.AllowInsecure {
		t.Error("Metrics.AllowInsecure should be true after --metrics-allow-insecure=true")
	}
	if err := got.Validate(); err != nil {
		t.Errorf("Validate with allow_insecure=true: %v", err)
	}
}
