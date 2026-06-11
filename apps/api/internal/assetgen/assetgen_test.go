// Package assetgen_test contains table-driven unit tests for the
// opc asset-generation primitives: the canonical panda IP profile,
// the prompt builders, and the brand-consistency check.
package assetgen_test

import (
	"strings"
	"testing"

	"github.com/opc/api/internal/assetgen"
)

// TestProfileFenggeV1 verifies GetProfile's known/unknown/empty
// behavior. The default fallback to FenggeV1 is intentional — the
// CLI uses "" as "the only profile we ship" so callers don't need
// to hardcode the version string.
func TestProfileFenggeV1(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		version     string
		wantNil     bool
		wantVersion string
		wantName    string
	}{
		{
			name:        "explicit fengge_v1",
			version:     "fengge_v1",
			wantNil:     false,
			wantVersion: "fengge_v1",
			wantName:    "峰哥",
		},
		{
			name:        "empty string defaults to FenggeV1",
			version:     "",
			wantNil:     false,
			wantVersion: "fengge_v1",
			wantName:    "峰哥",
		},
		{
			name:    "unknown version returns nil",
			version: "nonexistent",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := assetgen.GetProfile(tt.version)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("GetProfile(%q) = %+v, want nil", tt.version, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("GetProfile(%q) = nil, want non-nil *Profile", tt.version)
			}
			if got.Version != tt.wantVersion {
				t.Errorf("Version = %q, want %q", got.Version, tt.wantVersion)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

// TestBuildPromptImage verifies the image-prompt path of BuildPrompt:
// every brand anchor from FenggeV1 must appear, the scene and
// outfit must be threaded through, and the image tail must carry
// the aspect-ratio flag (but not the video-only flags).
func TestBuildPromptImage(t *testing.T) {
	t.Parallel()

	p := assetgen.FenggeV1
	const scene = "竹林小院"
	const outfit = "朱红"
	got := assetgen.BuildPrompt(p, scene, outfit, assetgen.TypeImage)

	// Required brand anchors.
	wantContains := []string{
		"峰哥",               // name
		"成年熊猫",             // species
		"rgb(245,240,225)",   // body white
		"rgb(26,26,26)",      // eye black
		"国潮",               // national-trend theme
		"不露爪",              // anti-claws
		"--ratio 9:16",       // image-only aspect ratio
		scene,                // scene must be threaded in
		outfit,               // outfit must be threaded in
	}

	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Errorf("BuildPrompt(image) missing %q in:\n%s", s, got)
		}
	}

	// At least one palette color must appear in the palette
	// substring section.
	paletteFound := false
	for _, c := range p.Palette {
		if strings.Contains(got, c) {
			paletteFound = true
			break
		}
	}
	if !paletteFound {
		t.Errorf("BuildPrompt(image) missing palette color in:\n%s", got)
	}

	// Video-only flags must NOT appear in the image variant.
	for _, banned := range []string{"--duration", "--watermark"} {
		if strings.Contains(got, banned) {
			t.Errorf("BuildPrompt(image) unexpectedly contains %q in:\n%s", banned, got)
		}
	}
}

// TestBuildPromptVideo verifies the video-prompt path of
// BuildPrompt: same brand anchors as the image variant, plus the
// video technical-flag tail.
func TestBuildPromptVideo(t *testing.T) {
	t.Parallel()

	p := assetgen.FenggeV1
	const scene = "竹林小院"
	const outfit = "翠绿"
	got := assetgen.BuildPrompt(p, scene, outfit, assetgen.TypeVideo)

	// Same brand anchors as the image variant must still appear.
	wantContains := []string{
		"峰哥",
		"成年熊猫",
		"rgb(245,240,225)",
		"rgb(26,26,26)",
		"国潮",
		"不露爪",
		scene,
		outfit,
		"--duration 5 --resolution 720p --ratio 9:16 --watermark true",
	}

	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Errorf("BuildPrompt(video) missing %q in:\n%s", s, got)
		}
	}
}

// TestConsistencyCheck verifies ConsistencyCheck's scoring across
// three meaningful cases:
//
//   1. A full prompt (round-tripped from BuildPrompt) scores ~1.0.
//      The "anti-AI tokens" anchor ("不要 AI 生成感") is NOT in
//      BuildPrompt's output, so the canonical-image score is
//      0.95, not 1.0.
//   2. A prompt with zero anchors scores 0.0 and fails all 7
//      checks.
//   3. A prompt with only the name scores exactly 0.20 and fails
//      the other 6 checks.
func TestConsistencyCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		prompt    string
		wantScore float64
		wantFails int
	}{
		{
			name:      "full prompt from BuildPrompt scores 0.95",
			prompt:    assetgen.BuildPrompt(assetgen.FenggeV1, "x", "y", assetgen.TypeImage),
			wantScore: 0.95,
			wantFails: 1, // missing "不要 AI 生成感"
		},
		{
			name:      "no anchors scores 0.0 and fails all 7",
			prompt:    "no anchors here",
			wantScore: 0.0,
			wantFails: 7,
		},
		{
			name:      "only name scores 0.20 and fails 6",
			prompt:    "this has 峰哥 but no other anchors",
			wantScore: 0.20,
			wantFails: 6,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := assetgen.ConsistencyCheck(assetgen.FenggeV1, tt.prompt)

			if got.Score != tt.wantScore {
				t.Errorf("Score = %v, want %v", got.Score, tt.wantScore)
			}
			if len(got.Fails) != tt.wantFails {
				t.Errorf("len(Fails) = %d, want %d (Fails: %v)", len(got.Fails), tt.wantFails, got.Fails)
			}
		})
	}
}
