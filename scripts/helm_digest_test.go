/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// Digest pinning is the chart's half of the deploy-by-digest contract
// (RELEASE.md, skaphos/fathom#330): fathomctl sets the three digests of its
// release, and a digest must replace the tag rather than be glued onto it.

const (
	testOperatorDigest  = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	testProbeDigest     = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	testNodeAgentDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
)

// managerImages returns the manager container image and its --probe-image and
// --node-agent-image args from a rendered chart.
func managerImages(t *testing.T, objects []map[string]any) (image, probe, nodeAgent string) {
	t.Helper()
	deploy := helmObject(t, objects, "Deployment")
	podSpec := helmMap(t, helmMap(t, helmMap(t, deploy, "spec"), "template"), "spec")
	containers, _ := podSpec["containers"].([]any)
	for _, c := range containers {
		container, _ := c.(map[string]any)
		if container["name"] != "manager" {
			continue
		}
		image, _ = container["image"].(string)
		args, _ := container["args"].([]any)
		for _, a := range args {
			arg, _ := a.(string)
			if v, ok := strings.CutPrefix(arg, "--probe-image="); ok {
				probe = v
			}
			if v, ok := strings.CutPrefix(arg, "--node-agent-image="); ok {
				nodeAgent = v
			}
		}
	}
	return image, probe, nodeAgent
}

func chartAppVersion(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(runtimeChart, "Chart.yaml"))
	if err != nil {
		t.Fatalf("read Chart.yaml: %v", err)
	}
	var chart struct {
		AppVersion string `json:"appVersion"`
	}
	if err := yaml.Unmarshal(raw, &chart); err != nil {
		t.Fatalf("decode Chart.yaml: %v", err)
	}
	return chart.AppVersion
}

func TestHelmImagesDefaultToTheAppVersionTag(t *testing.T) {
	objects, out, err := renderRuntimeChart(t)
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	tag := ":v" + chartAppVersion(t)
	image, probe, nodeAgent := managerImages(t, objects)
	for name, ref := range map[string]string{"manager": image, "--probe-image": probe, "--node-agent-image": nodeAgent} {
		if !strings.HasSuffix(ref, tag) || strings.Contains(ref, "@") {
			t.Errorf("%s = %q, want a %s tag reference", name, ref, tag)
		}
	}
}

func TestHelmDigestsReplaceTags(t *testing.T) {
	objects, out, err := renderRuntimeChart(t,
		"--set", "image.digest="+testOperatorDigest,
		"--set", "probeImage.digest="+testProbeDigest,
		"--set", "nodeAgent.image.digest="+testNodeAgentDigest,
		// A tag set alongside a digest is ignored: the digest is the identity.
		"--set", "image.tag=v9.9.9")
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	image, probe, nodeAgent := managerImages(t, objects)
	want := map[string][2]string{
		"manager":            {image, "ghcr.io/skaphos/fathom-operator@" + testOperatorDigest},
		"--probe-image":      {probe, "ghcr.io/skaphos/fathom-probe@" + testProbeDigest},
		"--node-agent-image": {nodeAgent, "ghcr.io/skaphos/fathom-node-agent@" + testNodeAgentDigest},
	}
	for name, got := range want {
		if got[0] != got[1] {
			t.Errorf("%s = %q, want %q", name, got[0], got[1])
		}
	}
}

func TestHelmRejectsMalformedDigests(t *testing.T) {
	for _, value := range []string{
		"image.digest=v0.6.0",
		"probeImage.digest=sha256:abc",
		"nodeAgent.image.digest=sha512:" + strings.Repeat("a", 128),
		"image.digest=sha256:" + strings.Repeat("A", 64),
	} {
		t.Run(value, func(t *testing.T) {
			_, out, err := renderRuntimeChart(t, "--set", value)
			if err == nil {
				t.Fatalf("helm template accepted %s", value)
			}
			if !strings.Contains(out, "digest") {
				t.Errorf("schema error does not name the digest field:\n%s", out)
			}
		})
	}
}
