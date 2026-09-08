/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFathomctlDistProducesVerifiableArchive runs scripts/fathomctl-dist.sh
// for the host platform and proves what the release workflow relies on: the
// archive exists, the checksums file names it with a matching SHA-256, and
// the extracted binary reports the injected version. It cross-compiles one
// binary, so it is skipped under -short.
func TestFathomctlDistProducesVerifiableArchive(t *testing.T) {
	if testing.Short() {
		t.Skip("builds fathomctl; skipped under -short")
	}
	out := t.TempDir()
	platform := runtime.GOOS + "/" + runtime.GOARCH
	cmd := exec.Command("bash", "fathomctl-dist.sh")
	cmd.Env = append(os.Environ(), "VERSION=0.0.0-test", "OUT="+out, "FATHOMCTL_PLATFORMS="+platform)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fathomctl-dist.sh failed: %v\n%s", err, output)
	}

	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	base := fmt.Sprintf("fathomctl_0.0.0-test_%s_%s", runtime.GOOS, runtime.GOARCH)
	archive := filepath.Join(out, base+ext)
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("archive missing: %v", err)
	}

	sums, err := os.Open(filepath.Join(out, "fathomctl_0.0.0-test_checksums.txt"))
	if err != nil {
		t.Fatalf("checksums file missing: %v", err)
	}
	defer func() { _ = sums.Close() }()
	var recorded string
	scanner := bufio.NewScanner(sums)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == base+ext {
			recorded = fields[0]
		}
	}
	if recorded == "" {
		t.Fatalf("checksums file does not name %s", base+ext)
	}
	if actual := fileSHA256(t, archive); actual != recorded {
		t.Fatalf("checksum mismatch: recorded %s, actual %s", recorded, actual)
	}

	if runtime.GOOS == "windows" {
		return
	}
	extract := exec.Command("tar", "-xzf", archive, "-C", out)
	if output, err := extract.CombinedOutput(); err != nil {
		t.Fatalf("extract: %v\n%s", err, output)
	}
	bin := filepath.Join(out, base, "fathomctl")
	version, err := exec.Command(bin, "version", "--client").Output()
	if err != nil {
		t.Fatalf("run extracted binary: %v", err)
	}
	if !strings.Contains(string(version), "v0.0.0-test") {
		t.Fatalf("binary reports %q, want v0.0.0-test", strings.TrimSpace(string(version)))
	}
	if _, err := os.Stat(filepath.Join(out, base, "LICENSE")); err != nil {
		t.Fatalf("archive must include LICENSE: %v", err)
	}
}

func TestFathomctlDistRequiresVersion(t *testing.T) {
	cmd := exec.Command("bash", "fathomctl-dist.sh")
	cmd.Env = append(os.Environ(), "VERSION=", "OUT="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure without VERSION:\n%s", output)
	}
	if !strings.Contains(string(output), "VERSION is required") {
		t.Fatalf("expected a VERSION diagnostic, got:\n%s", output)
	}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
