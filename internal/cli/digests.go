/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

package cli

import (
	"fmt"
	"sort"
	"strings"
)

// Image digests of the release this fathomctl belongs to, injected by the
// release workflow (scripts/fathomctl-dist.sh) alongside Version:
//
//	-X github.com/skaphos/fathom/internal/cli.operatorDigest=sha256:…
//
// A build without them — anything not produced by the release workflow — is
// a development build and installs by tag instead (spec 013 Assumptions).
var (
	operatorDigest  string
	probeDigest     string
	nodeAgentDigest string
)

// The image repositories every release publishes to. They are fixed by the
// release, like the digests, so an install can never mix images.
const (
	operatorRepository  = "ghcr.io/skaphos/fathom-operator"
	probeRepository     = "ghcr.io/skaphos/fathom-probe"
	nodeAgentRepository = "ghcr.io/skaphos/fathom-node-agent"
)

// imagePinning is how the installed images are referenced.
type imagePinning string

const (
	pinnedByDigest imagePinning = "digest"
	pinnedByTag    imagePinning = "tag"
)

// pinning reports whether this build carries its release digests. All three
// are injected together; a partial set is treated as a development build.
func pinning() imagePinning {
	if operatorDigest != "" && probeDigest != "" && nodeAgentDigest != "" {
		return pinnedByDigest
	}
	return pinnedByTag
}

// imageValues returns the chart values for the three images of this
// release. It always sets repository, tag and digest, so image values a
// previous revision recorded (another release's digests, a tag a Helm user
// set) never survive a reuse-values upgrade (FR-007). version is the chart
// version, without a leading v.
func imageValues(version string) map[string]any {
	tag := "v" + strings.TrimPrefix(version, "v")
	digests := [3]string{}
	if pinning() == pinnedByDigest {
		digests = [3]string{operatorDigest, probeDigest, nodeAgentDigest}
	}
	return map[string]any{
		"image":      map[string]any{"repository": operatorRepository, "tag": tag, "digest": digests[0]},
		"probeImage": map[string]any{"repository": probeRepository, "tag": tag, "digest": digests[1]},
		"nodeAgent": map[string]any{
			"image": map[string]any{"repository": nodeAgentRepository, "tag": tag, "digest": digests[2]},
		},
	}
}

// imageReferences renders the references imageValues produces, for plans and
// summaries.
func imageReferences(version string) map[string]string {
	refs := map[string]string{}
	tag := "v" + strings.TrimPrefix(version, "v")
	for _, img := range []struct{ name, repo, digest string }{
		{"operator", operatorRepository, operatorDigest},
		{"probe", probeRepository, probeDigest},
		{"node-agent", nodeAgentRepository, nodeAgentDigest},
	} {
		if pinning() == pinnedByDigest {
			refs[img.name] = img.repo + "@" + img.digest
		} else {
			refs[img.name] = img.repo + ":" + tag
		}
	}
	return refs
}

// imageValueKeys are the value paths the CLI owns. pullPolicy is deliberately
// absent: how an image is pulled does not change which image runs.
var imageValueKeys = [][]string{
	{"image", "repository"}, {"image", "tag"}, {"image", "digest"},
	{"probeImage", "repository"}, {"probeImage", "tag"}, {"probeImage", "digest"},
	{"nodeAgent", "image", "repository"}, {"nodeAgent", "image", "tag"}, {"nodeAgent", "image", "digest"},
}

// rejectImageOverrides fails when user-supplied values set any image the CLI
// owns. Each fathomctl installs exactly its own release's images; a different
// image means running that release's fathomctl (FR-003, FR-007).
func rejectImageOverrides(values map[string]any) error {
	var set []string
	for _, key := range imageValueKeys {
		if _, ok := lookupValue(values, key); ok {
			set = append(set, strings.Join(key, "."))
		}
	}
	if len(set) == 0 {
		return nil
	}
	sort.Strings(set)
	return usageError{fmt.Errorf("image values cannot be overridden (%s): each fathomctl installs exactly its own release's images, so another image means running that release's fathomctl", strings.Join(set, ", "))}
}

func lookupValue(values map[string]any, key []string) (any, bool) {
	var cur any = values
	for _, k := range key {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// usageError marks a refusal caused by the flags themselves rather than the
// cluster, so callers can say so; it still exits 1 like every error.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }
