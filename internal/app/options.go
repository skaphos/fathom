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
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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
}

// WebhookOptions configures the controller manager's webhook server.
type WebhookOptions struct {
	CertPath string `mapstructure:"cert_path"`
	CertName string `mapstructure:"cert_name"`
	CertKey  string `mapstructure:"cert_key"`
}

// Options is the resolved runtime configuration for the operator.
type Options struct {
	Metrics                MetricsOptions `mapstructure:"metrics"`
	Webhook                WebhookOptions `mapstructure:"webhook"`
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
			BindAddress: "0",
			Secure:      true,
			CertName:    "tls.crt",
			CertKey:     "tls.key",
		},
		Webhook: WebhookOptions{
			CertName: "tls.crt",
			CertKey:  "tls.key",
		},
		HealthProbeBindAddress: ":8081",
		LeaderElect:            false,
		LeaderElectionID:       "2d3dbc4f.skaphos.io",
		EnableHTTP2:            false,
		ProbeImage:             DefaultProbeImage,
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
	return errors.Join(errs...)
}

// flagBinding pairs a flag name with its corresponding Viper key. Keeping them
// in one table guarantees flags, env vars, and config file keys stay in sync.
type flagBinding struct {
	flagName  string
	viperKey  string
	usage     string
	stringDef string
	boolDef   bool
	isBool    bool
}

func bindings(defaults Options) []flagBinding {
	return []flagBinding{
		{flagName: "metrics-bind-address", viperKey: "metrics.bind_address", stringDef: defaults.Metrics.BindAddress,
			usage: "The address the metrics endpoint binds to. Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service."},
		{flagName: "metrics-secure", viperKey: "metrics.secure", isBool: true, boolDef: defaults.Metrics.Secure,
			usage: "If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead."},
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
	}
}

// RegisterFlags registers operator flags onto fs and zap.Options flags onto
// zapOpts. Flags are not pointer-bound: values are resolved later by Viper
// using each flag's Changed state to honor precedence.
func RegisterFlags(fs *pflag.FlagSet, zapOpts *zap.Options) {
	defaults := DefaultOptions()
	for _, b := range bindings(defaults) {
		if b.isBool {
			fs.Bool(b.flagName, b.boolDef, b.usage)
		} else {
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
		if b.isBool {
			v.SetDefault(b.viperKey, b.boolDef)
		} else {
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
	if err := v.Unmarshal(&opts); err != nil {
		return Options{}, fmt.Errorf("unmarshal config: %w", err)
	}
	opts.Zap = zapOpts
	return opts, nil
}
