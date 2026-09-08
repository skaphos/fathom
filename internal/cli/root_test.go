/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// TestRootCommand_GlobalFlags locks the persistent flag surface: every global
// flag parses into globalOptions, and the defaults match the documented
// contract (table output, 30s request timeout, context namespace).
func TestRootCommand_GlobalFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want globalOptions
	}{
		{
			name: "defaults",
			args: nil,
			want: globalOptions{output: "", requestTimeout: defaultRequestTimeout},
		},
		{
			name: "all long flags",
			args: []string{"--kubeconfig", "/tmp/kc", "--context", "prod", "--namespace", "team-a", "--output", "json", "--request-timeout", "5s"},
			want: globalOptions{kubeconfig: "/tmp/kc", context: "prod", namespace: "team-a", output: outputJSON, requestTimeout: 5 * time.Second},
		},
		{
			name: "short flags",
			args: []string{"-n", "team-b", "-o", "yaml"},
			want: globalOptions{namespace: "team-b", output: outputYAML, requestTimeout: defaultRequestTimeout},
		},
		{
			name: "all namespaces",
			args: []string{"-A"},
			want: globalOptions{allNamespaces: true, requestTimeout: defaultRequestTimeout},
		},
		{
			name: "output is case-insensitive",
			args: []string{"-o", "JSON"},
			want: globalOptions{output: outputJSON, requestTimeout: defaultRequestTimeout},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFactory()
			cmd := newRootCommand(f)
			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("ParseFlags(%v): %v", tt.args, err)
			}
			if *f.opts != tt.want {
				t.Fatalf("options = %+v, want %+v", *f.opts, tt.want)
			}
		})
	}
}

// withNoopVerb registers a runnable verb so PersistentPreRunE fires: cobra
// only validates on a runnable command, and a bare root prints help.
func withNoopVerb(cmd *cobra.Command) *cobra.Command {
	cmd.AddCommand(&cobra.Command{Use: "noop", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }})
	return cmd
}

// TestRootCommand_RejectsBadFlags covers the parse-time and validate-time
// usage errors: an unsupported -o value is rejected by the pflag.Value, and
// -n with -A or a negative timeout is rejected by PersistentPreRunE before
// any verb runs.
func TestRootCommand_RejectsBadFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unsupported output", []string{"noop", "-o", "xml"}, `unsupported output format "xml"`},
		{"namespace with all-namespaces", []string{"noop", "-n", "a", "-A"}, "mutually exclusive"},
		{"negative timeout", []string{"noop", "--request-timeout", "-1s"}, "must not be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFactory()
			cmd := withNoopVerb(newRootCommand(f))
			cmd.SetArgs(tt.args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Execute(%v) error = %v, want containing %q", tt.args, err, tt.wantErr)
			}
		})
	}
}

// TestRootCommand_HelpMentionsExitCodes keeps the kubectl exit-code
// convention visible to users without opening the docs.
func TestRootCommand_HelpMentionsExitCodes(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommand(newFactory())
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out.String(), "0 on success, 1 on any error") {
		t.Fatalf("help text should state the exit-code convention; got:\n%s", out.String())
	}
}

// TestRootCommand_VerbSet locks the verb surface: exactly the verbs that have
// landed plus cobra's completion and help, and never pause or resume.
func TestRootCommand_VerbSet(t *testing.T) {
	cmd := newRootCommand(newFactory())
	var got []string
	for _, c := range cmd.Commands() {
		got = append(got, c.Name())
	}
	want := map[string]bool{"ls": true, "describe": true, "reports": true, "run": true, "version": true, "completion": true, "help": true}
	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected verb %q registered", name)
		}
		delete(want, name)
	}
	for missing := range want {
		t.Errorf("verb %q not registered", missing)
	}
	for _, forbidden := range []string{"pause", "resume"} {
		if c, _, err := cmd.Find([]string{forbidden}); err == nil && c != cmd {
			t.Errorf("%s must not exist (decision #262)", forbidden)
		}
	}
}

func TestGlobalOptions_Validate(t *testing.T) {
	ok := globalOptions{namespace: "a", requestTimeout: time.Second}
	if err := ok.validate(); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}
	both := globalOptions{namespace: "a", allNamespaces: true, requestTimeout: -time.Second}
	err := both.validate()
	if err == nil {
		t.Fatal("expected an error for -n with -A and a negative timeout")
	}
	for _, want := range []string{"mutually exclusive", "must not be negative"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validate() error %q should mention %q (errors must be joined, not first-wins)", err, want)
		}
	}
}
