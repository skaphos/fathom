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
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// A duration binding resolves through the same Viper pipeline as every other
// kind: flag beats env beats config file beats default, env and config values
// arrive as strings, and "0" round-trips to a zero duration.
func TestDurationBindingPrecedenceAndParsing(t *testing.T) {
	binding := flagBinding{
		flagName: "window", viperKey: "sample.window", isDuration: true,
		durationDef: 720 * time.Hour, usage: "test duration",
	}
	type sample struct {
		Window time.Duration `mapstructure:"window"`
	}
	type cfg struct {
		Sample sample `mapstructure:"sample"`
	}

	configPath := filepath.Join(t.TempDir(), "fathom.yaml")
	if err := os.WriteFile(configPath, []byte("sample:\n  window: 36h\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	load := func(t *testing.T, args []string, configFile string) time.Duration {
		t.Helper()
		fs := pflag.NewFlagSet("duration-test", pflag.ContinueOnError)
		registerFlag(fs, binding)
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		v := viper.New()
		v.SetEnvPrefix(EnvPrefix)
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()
		v.SetDefault(binding.viperKey, bindingDefault(binding))
		if err := v.BindPFlag(binding.viperKey, fs.Lookup(binding.flagName)); err != nil {
			t.Fatal(err)
		}
		if configFile != "" {
			v.SetConfigFile(configFile)
			if err := v.ReadInConfig(); err != nil {
				t.Fatal(err)
			}
		}
		var out cfg
		weaklyTyped := func(c *mapstructure.DecoderConfig) { c.WeaklyTypedInput = true }
		if err := v.Unmarshal(&out, weaklyTyped); err != nil {
			t.Fatal(err)
		}
		return out.Sample.Window
	}

	t.Run("default when nothing set", func(t *testing.T) {
		if got := load(t, nil, ""); got != 720*time.Hour {
			t.Fatalf("got %s, want 720h", got)
		}
	})
	t.Run("config beats default", func(t *testing.T) {
		if got := load(t, nil, configPath); got != 36*time.Hour {
			t.Fatalf("got %s, want 36h", got)
		}
	})
	t.Run("env beats config", func(t *testing.T) {
		t.Setenv("FATHOM_SAMPLE_WINDOW", "12h")
		if got := load(t, nil, configPath); got != 12*time.Hour {
			t.Fatalf("got %s, want 12h", got)
		}
	})
	t.Run("flag beats env", func(t *testing.T) {
		t.Setenv("FATHOM_SAMPLE_WINDOW", "12h")
		if got := load(t, []string{"--window=90m"}, configPath); got != 90*time.Minute {
			t.Fatalf("got %s, want 90m", got)
		}
	})
	t.Run("zero round-trips", func(t *testing.T) {
		t.Setenv("FATHOM_SAMPLE_WINDOW", "0")
		if got := load(t, nil, configPath); got != 0 {
			t.Fatalf("got %s, want 0", got)
		}
	})
}

// The registered flag carries the binding's default and usage, so --help and
// DefaultOptions agree the way they do for every other binding kind.
func TestDurationBindingRegistersADurationFlag(t *testing.T) {
	fs := pflag.NewFlagSet("duration-test", pflag.ContinueOnError)
	registerFlag(fs, flagBinding{flagName: "window", viperKey: "sample.window", isDuration: true, durationDef: time.Hour, usage: "u"})
	f := fs.Lookup("window")
	if f == nil || f.Value.Type() != "duration" || f.DefValue != "1h0m0s" || f.Usage != "u" {
		t.Fatalf("flag = %+v, want a duration flag defaulting to 1h", f)
	}
}
