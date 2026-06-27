/*
SPDX-FileCopyrightText: 2026 Skaphos
SPDX-License-Identifier: MIT
*/

// Package app builds and runs the Fathom controller manager. Configuration is
// resolved from defaults, an optional config file, environment variables, and
// command-line flags (in increasing order of precedence) via Viper.
package app

import (
	"errors"
	"flag"
	"fmt"
	iofs "io/fs"
	"net"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// EnvPrefix is the prefix Viper uses when reading environment variables.
// E.g. metrics.bind_address is read from FATHOM_METRICS_BIND_ADDRESS.
const EnvPrefix = "FATHOM"

// DefaultConfigPath is the default location for a YAML/JSON/TOML config file.
// Operators typically mount a ConfigMap here.
const DefaultConfigPath = "/etc/fathom/config.yaml"

// MetricsOptions configures the controller manager's metrics server.
type MetricsOptions struct {
	BindAddress string `mapstructure:"bind_address"`
	Secure      bool   `mapstructure:"secure"`
	CertPath    string `mapstructure:"cert_path"`
	CertName    string `mapstructure:"cert_name"`
	CertKey     string `mapstructure:"cert_key"`
	// AllowInsecure is an explicit opt-in to expose the metrics endpoint over
	// plaintext HTTP on a cluster-routable port (BindAddress != "0" with
	// Secure=false). Defaults to false: Validate rejects insecure-on-network
	// otherwise. Intended for operators fronting the endpoint with a service
	// mesh (mTLS, authz at the proxy).
	AllowInsecure bool `mapstructure:"allow_insecure"`
}

// WebhookOptions configures the controller manager's webhook server.
type WebhookOptions struct {
	CertPath string `mapstructure:"cert_path"`
	CertName string `mapstructure:"cert_name"`
	CertKey  string `mapstructure:"cert_key"`
}

// TracingOptions configures OpenTelemetry trace export for the operator
// (SKA-293). When Enabled is false the operator installs a no-op tracer
// provider and never builds an exporter, so the reconcile/adapter hot paths
// carry ~zero tracing overhead.
type TracingOptions struct {
	// Enabled turns on span creation and OTLP export. Off by default.
	Enabled bool `mapstructure:"enabled"`
	// OTLPEndpoint is the gRPC OTLP collector endpoint (host:port). Empty
	// defers to the OTel SDK default (localhost:4317) and the standard
	// OTEL_EXPORTER_OTLP_* environment variables.
	OTLPEndpoint string `mapstructure:"otlp_endpoint"`
	// SamplingRatio is the head-based sampling probability in [0,1] applied by
	// a parent-based TraceIDRatioBased sampler. 1.0 samples every root span.
	SamplingRatio float64 `mapstructure:"sampling_ratio"`
	// Insecure disables transport security (plaintext gRPC) to the collector.
	// Intended for in-cluster collectors fronted by a service mesh.
	Insecure bool `mapstructure:"insecure"`
}

// Options is the resolved runtime configuration for the operator.
type Options struct {
	Metrics                MetricsOptions `mapstructure:"metrics"`
	Webhook                WebhookOptions `mapstructure:"webhook"`
	Tracing                TracingOptions `mapstructure:"tracing"`
	HealthProbeBindAddress string         `mapstructure:"health_probe_bind_address"`
	LeaderElect            bool           `mapstructure:"leader_elect"`
	LeaderElectionID       string         `mapstructure:"leader_election_id"`
	EnableHTTP2            bool           `mapstructure:"enable_http2"`

	// ProbeImage is the cluster-wide default container image for adapter
	// probe pods (see internal/probe). Adapters consult adapter.Request.ProbeImage
	// when no per-AddonCheck override is set. Operators running a private
	// GHCR mirror set this once instead of on every AddonCheck.
	ProbeImage string `mapstructure:"probe_image"`

	// Zap is bound directly to flags (not Viper) because zap.Options uses
	// the stdlib flag package and is not round-trippable via mapstructure.
	// Its default is whatever zap.Options.BindFlags registers (Development
	// defaults to false), not what DefaultOptions returns — Load overwrites
	// opts.Zap with the flag-parsed value before returning.
	Zap zap.Options `mapstructure:"-"`
}

// DefaultProbeImage is the published probe image this build of Fathom ships
// with. Probe-using adapters fall back to this when neither a per-AddonCheck
// threshold nor an operator-level --probe-image flag is set. Bumped in lockstep
// with the probe-image publish pipeline.
const DefaultProbeImage = "ghcr.io/skaphos/fathom-probe:v0.0.2"

// DefaultOptions returns Options pre-populated with the operator's defaults.
// These match the values registered as flag and Viper defaults.
//
// Zap is intentionally left zero — its default is owned by zap.Options.BindFlags
// (Development=false). Setting Zap here would be dead code since Load overwrites
// opts.Zap with the flag-parsed value before returning.
func DefaultOptions() Options {
	return Options{
		Metrics: MetricsOptions{
			BindAddress:   "0",
			Secure:        true,
			CertName:      "tls.crt",
			CertKey:       "tls.key",
			AllowInsecure: false,
		},
		Webhook: WebhookOptions{
			CertName: "tls.crt",
			CertKey:  "tls.key",
		},
		Tracing: TracingOptions{
			Enabled:       false,
			SamplingRatio: 1.0,
			Insecure:      false,
		},
		HealthProbeBindAddress: ":8081",
		// SKA-303: default leader election on so a multi-replica deployment that
		// drops the --leader-elect arg can't silently run duplicate reconcilers.
		LeaderElect:      true,
		LeaderElectionID: "2d3dbc4f.skaphos.io",
		EnableHTTP2:      false,
		ProbeImage:       DefaultProbeImage,
	}
}

// Validate returns an error if the resolved options are internally inconsistent.
func (o *Options) Validate() error {
	var errs []error
	if o.Webhook.CertPath != "" {
		if o.Webhook.CertName == "" {
			errs = append(errs, errors.New("webhook.cert_name must be set when webhook.cert_path is set"))
		}
		if o.Webhook.CertKey == "" {
			errs = append(errs, errors.New("webhook.cert_key must be set when webhook.cert_path is set"))
		}
	}
	if o.Metrics.CertPath != "" {
		if o.Metrics.CertName == "" {
			errs = append(errs, errors.New("metrics.cert_name must be set when metrics.cert_path is set"))
		}
		if o.Metrics.CertKey == "" {
			errs = append(errs, errors.New("metrics.cert_key must be set when metrics.cert_path is set"))
		}
	}
	if o.HealthProbeBindAddress == "" {
		errs = append(errs, errors.New("health_probe_bind_address must not be empty"))
	}

	// SKA-287: refuse to serve metrics in the clear on a cluster-routable port.
	// "0" disables the metrics server entirely (no listener); any other value is
	// a real bind address. Without Metrics.Secure, the filter-provider chain in
	// run.go skips TokenReview/SubjectAccessReview, so anyone with network reach
	// to the pod can scrape. AllowInsecure is the explicit opt-in for mesh-
	// fronted deployments.
	if o.Metrics.BindAddress != "0" && !o.Metrics.Secure && !o.Metrics.AllowInsecure {
		errs = append(errs, errors.New(
			"metrics.bind_address is set and metrics.secure=false: refusing to expose "+
				"plaintext metrics on a cluster-routable port. Set metrics.secure=true, "+
				"set metrics.bind_address=0, or set metrics.allow_insecure=true to opt in "+
				"(e.g. when fronted by a service mesh)",
		))
	}

	// SKA-299: surface address/ID syntax errors at Validate time instead of
	// deferring them to mgr.Start.
	if o.Metrics.BindAddress != "0" {
		if _, _, err := net.SplitHostPort(o.Metrics.BindAddress); err != nil {
			errs = append(errs, fmt.Errorf("metrics.bind_address %q is not a valid host:port: %w", o.Metrics.BindAddress, err))
		}
	}
	if o.HealthProbeBindAddress != "" && o.HealthProbeBindAddress != "0" {
		if _, _, err := net.SplitHostPort(o.HealthProbeBindAddress); err != nil {
			errs = append(errs, fmt.Errorf("health_probe_bind_address %q is not a valid host:port (use \"0\" to disable): %w", o.HealthProbeBindAddress, err))
		}
	}
	if o.LeaderElectionID != "" {
		if msgs := validation.IsDNS1123Subdomain(o.LeaderElectionID); len(msgs) > 0 {
			errs = append(errs, fmt.Errorf("leader_election_id %q is not a valid DNS-1123 subdomain: %s", o.LeaderElectionID, strings.Join(msgs, "; ")))
		}
	}

	// SKA-293: a sampling ratio outside [0,1] is almost certainly a typo. The
	// SDK would clamp it silently; rejecting it surfaces the mistake.
	if o.Tracing.SamplingRatio < 0 || o.Tracing.SamplingRatio > 1 {
		errs = append(errs, fmt.Errorf("tracing.sampling_ratio %v is out of range; must be between 0 and 1", o.Tracing.SamplingRatio))
	}

	return errors.Join(errs...)
}

// flagBinding pairs a flag name with its corresponding Viper key. Keeping them
// in one table guarantees flags, env vars, and config file keys stay in sync.
//
// At most one of isBool/isFloat is set; otherwise the binding registers a
// string flag with stringDef as its default.
type flagBinding struct {
	flagName  string
	viperKey  string
	usage     string
	stringDef string
	boolDef   bool
	isBool    bool
	floatDef  float64
	isFloat   bool
}

func bindings(defaults Options) []flagBinding {
	return []flagBinding{
		{flagName: "metrics-bind-address", viperKey: "metrics.bind_address", stringDef: defaults.Metrics.BindAddress,
			usage: "The address the metrics endpoint binds to. Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service."},
		{flagName: "metrics-secure", viperKey: "metrics.secure", isBool: true, boolDef: defaults.Metrics.Secure,
			usage: "If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead."},
		{flagName: "metrics-allow-insecure", viperKey: "metrics.allow_insecure", isBool: true, boolDef: defaults.Metrics.AllowInsecure,
			usage: "Explicitly allow serving metrics over plaintext HTTP on a cluster-routable port (i.e. --metrics-secure=false with --metrics-bind-address not 0). Intended for service-mesh-fronted deployments. Off by default; Validate rejects insecure-on-network otherwise."},
		{flagName: "metrics-cert-path", viperKey: "metrics.cert_path", stringDef: defaults.Metrics.CertPath,
			usage: "The directory that contains the metrics server certificate."},
		{flagName: "metrics-cert-name", viperKey: "metrics.cert_name", stringDef: defaults.Metrics.CertName,
			usage: "The name of the metrics server certificate file."},
		{flagName: "metrics-cert-key", viperKey: "metrics.cert_key", stringDef: defaults.Metrics.CertKey,
			usage: "The name of the metrics server key file."},
		{flagName: "webhook-cert-path", viperKey: "webhook.cert_path", stringDef: defaults.Webhook.CertPath,
			usage: "The directory that contains the webhook certificate."},
		{flagName: "webhook-cert-name", viperKey: "webhook.cert_name", stringDef: defaults.Webhook.CertName,
			usage: "The name of the webhook certificate file."},
		{flagName: "webhook-cert-key", viperKey: "webhook.cert_key", stringDef: defaults.Webhook.CertKey,
			usage: "The name of the webhook key file."},
		{flagName: "health-probe-bind-address", viperKey: "health_probe_bind_address", stringDef: defaults.HealthProbeBindAddress,
			usage: "The address the probe endpoint binds to."},
		{flagName: "leader-elect", viperKey: "leader_elect", isBool: true, boolDef: defaults.LeaderElect,
			usage: "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager."},
		{flagName: "leader-election-id", viperKey: "leader_election_id", stringDef: defaults.LeaderElectionID,
			usage: "The name of the resource used for leader election."},
		{flagName: "enable-http2", viperKey: "enable_http2", isBool: true, boolDef: defaults.EnableHTTP2,
			usage: "If set, HTTP/2 will be enabled for the metrics and webhook servers."},
		{flagName: "probe-image", viperKey: "probe_image", stringDef: defaults.ProbeImage,
			usage: "Container image used by adapters that launch probe pods. Per-AddonCheck thresholds still override this."},
		{flagName: "tracing-enabled", viperKey: "tracing.enabled", isBool: true, boolDef: defaults.Tracing.Enabled,
			usage: "Enable OpenTelemetry tracing of reconciles and adapter runs, exported via OTLP/gRPC. Off by default (no-op tracer, ~zero overhead)."},
		{flagName: "tracing-otlp-endpoint", viperKey: "tracing.otlp_endpoint", stringDef: defaults.Tracing.OTLPEndpoint,
			usage: "OTLP/gRPC collector endpoint (host:port) for trace export. Empty uses the OTel SDK default (localhost:4317) and the standard OTEL_EXPORTER_OTLP_* env vars."},
		{flagName: "tracing-sampling-ratio", viperKey: "tracing.sampling_ratio", isFloat: true, floatDef: defaults.Tracing.SamplingRatio,
			usage: "Head-based trace sampling probability in [0,1] for a parent-based ratio sampler. 1.0 samples every root span."},
		{flagName: "tracing-insecure", viperKey: "tracing.insecure", isBool: true, boolDef: defaults.Tracing.Insecure,
			usage: "Disable transport security (plaintext gRPC) to the OTLP collector. Intended for in-cluster collectors fronted by a service mesh."},
	}
}

// RegisterFlags registers operator flags onto fs and zap.Options flags onto
// zapOpts. Flags are not pointer-bound: values are resolved later by Viper
// using each flag's Changed state to honor precedence.
func RegisterFlags(fs *pflag.FlagSet, zapOpts *zap.Options) {
	defaults := DefaultOptions()
	for _, b := range bindings(defaults) {
		switch {
		case b.isBool:
			fs.Bool(b.flagName, b.boolDef, b.usage)
		case b.isFloat:
			fs.Float64(b.flagName, b.floatDef, b.usage)
		default:
			fs.String(b.flagName, b.stringDef, b.usage)
		}
	}

	// Bridge zap.Options' stdlib flags into our pflag set so cobra can parse them.
	// They are NOT routed through Viper; zapOpts is mutated directly.
	goFS := flag.NewFlagSet("zap", flag.ContinueOnError)
	zapOpts.BindFlags(goFS)
	fs.AddGoFlagSet(goFS)
}

// Load resolves Options using Viper. Precedence is flag > env > config file > default.
//
//   - fs must already be parsed (e.g. by cobra) so that flag.Changed is meaningful.
//   - zapOpts holds any zap configuration parsed from flags; it is copied verbatim.
//   - configFile, if non-empty, is read as the operator's config file. A missing
//     file at the default path is treated as "no config", but a missing file at
//     an explicitly-set path returns an error.
func Load(fs *pflag.FlagSet, zapOpts zap.Options, configFile string, configExplicit bool) (Options, error) {
	v := viper.New()
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	defaults := DefaultOptions()
	for _, b := range bindings(defaults) {
		switch {
		case b.isBool:
			v.SetDefault(b.viperKey, b.boolDef)
		case b.isFloat:
			v.SetDefault(b.viperKey, b.floatDef)
		default:
			v.SetDefault(b.viperKey, b.stringDef)
		}
		if f := fs.Lookup(b.flagName); f != nil {
			if err := v.BindPFlag(b.viperKey, f); err != nil {
				return Options{}, fmt.Errorf("bind flag %q: %w", b.flagName, err)
			}
		}
	}

	if configFile != "" {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			notFound := errors.Is(err, iofs.ErrNotExist)
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
				notFound = true
			}
			if !notFound || configExplicit {
				return Options{}, fmt.Errorf("read config %q: %w", configFile, err)
			}
		}
	}

	opts := defaults
	// WeaklyTypedInput lets scalar options arrive as strings (the only shape an
	// environment variable can take) and still decode into bool/float fields —
	// e.g. FATHOM_TRACING_ENABLED=true or FATHOM_TRACING_SAMPLING_RATIO=0.5.
	// viper applies its default decode hooks first, so this only relaxes scalar
	// coercion; it leaves nested struct/slice decoding untouched.
	weaklyTyped := func(c *mapstructure.DecoderConfig) { c.WeaklyTypedInput = true }
	if err := v.Unmarshal(&opts, weaklyTyped); err != nil {
		return Options{}, fmt.Errorf("unmarshal config: %w", err)
	}
	opts.Zap = zapOpts
	return opts, nil
}
