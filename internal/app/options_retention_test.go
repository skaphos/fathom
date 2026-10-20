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
)

// The retention options (spec 014, contracts/retention.md) follow the normal
// precedence and refuse values that would make retention meaningless.
func TestLoad_RetentionPrecedence(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "fathom.yaml")
	if err := os.WriteFile(configPath, []byte("retention:\n  report_window: 36h\n  report_cap_per_kind: 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	load := func(t *testing.T, args []string, config string) RetentionOptions {
		t.Helper()
		fs, zapOpts := newTestFlags(t)
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		got, err := Load(fs, *zapOpts, config, config != "")
		if err != nil {
			t.Fatal(err)
		}
		return got.Retention
	}
	t.Run("defaults", func(t *testing.T) {
		got := load(t, nil, "")
		if got.ReportWindow != DefaultRetentionReportWindow || got.ReportCapPerKind != DefaultRetentionReportCapPerKind {
			t.Fatalf("got %+v, want 720h / 500", got)
		}
		if DefaultRetentionReportWindow != 30*24*time.Hour {
			t.Fatalf("default window = %s, want 30 days", DefaultRetentionReportWindow)
		}
	})
	t.Run("config beats default", func(t *testing.T) {
		got := load(t, nil, configPath)
		if got.ReportWindow != 36*time.Hour || got.ReportCapPerKind != 7 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("env beats config", func(t *testing.T) {
		t.Setenv("FATHOM_RETENTION_REPORT_WINDOW", "12h")
		t.Setenv("FATHOM_RETENTION_REPORT_CAP_PER_KIND", "9")
		got := load(t, nil, configPath)
		if got.ReportWindow != 12*time.Hour || got.ReportCapPerKind != 9 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("flag beats env", func(t *testing.T) {
		t.Setenv("FATHOM_RETENTION_REPORT_WINDOW", "12h")
		got := load(t, []string{"--retention-report-window=0", "--retention-report-cap-per-kind=3"}, configPath)
		if got.ReportWindow != 0 || got.ReportCapPerKind != 3 {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestValidate_Retention(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*Options)
		want   string
	}{
		"negative window": {func(o *Options) { o.Retention.ReportWindow = -time.Minute }, "retention.report_window"},
		"zero cap":        {func(o *Options) { o.Retention.ReportCapPerKind = 0 }, "retention.report_cap_per_kind"},
	} {
		t.Run(name, func(t *testing.T) {
			opts := DefaultOptions()
			tc.mutate(&opts)
			err := opts.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want mention of %s", err, tc.want)
			}
		})
	}
	opts := DefaultOptions()
	opts.Retention.ReportWindow = 0
	if err := opts.Validate(); err != nil {
		t.Fatalf("a zero window is valid (prune at once): %v", err)
	}
}
