package anthropomorphic_test

import (
	"strings"
	"testing"

	"github.com/opc/api/internal/assetgen"
	"github.com/opc/api/internal/assetgen/profiles/anthropomorphic"
)

func TestBuildAnthropomorphicImage(t *testing.T) {
	t.Parallel()

	p := anthropomorphic.DefaultInstance()
	got := anthropomorphic.BuildPrompt(p, "竹林小院", "朱红", assetgen.TypeImage)

	wantContains := []string{
		"峰哥", "成年熊猫", "rgb(245,240,225)", "rgb(26,26,26)",
		"国潮", "不露爪", "不要 AI 生成感",
		"竹林小院", "朱红",
		"--ratio 9:16",
	}
	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Errorf("BuildPrompt(image) missing %q", s)
		}
	}
	for _, banned := range []string{"--duration", "--watermark"} {
		if strings.Contains(got, banned) {
			t.Errorf("BuildPrompt(image) unexpectedly contains %q", banned)
		}
	}
}

func TestBuildAnthropomorphicVideo(t *testing.T) {
	t.Parallel()

	p := anthropomorphic.DefaultInstance()
	got := anthropomorphic.BuildPrompt(p, "竹林小院", "翠绿", assetgen.TypeVideo)

	wantContains := []string{
		"峰哥", "成年熊猫", "rgb(245,240,225)", "rgb(26,26,26)",
		"国潮", "不露爪", "不要 AI 生成感",
		"竹林小院", "翠绿",
		"--duration 5 --resolution 720p --ratio 9:16 --watermark true",
	}
	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Errorf("BuildPrompt(video) missing %q", s)
		}
	}
}

func TestBuildAnthropomorphicNil(t *testing.T) {
	t.Parallel()

	got := anthropomorphic.BuildPrompt(nil, "scene", "outfit", assetgen.TypeImage)
	if got != "" {
		t.Errorf("nil profile got %q, want empty", got)
	}
}

func TestBuildAnthropomorphicWrongType(t *testing.T) {
	t.Parallel()

	bogus := &bogusProfile{typeName: "anthropomorphic"}
	got := anthropomorphic.BuildPrompt(bogus, "scene", "outfit", assetgen.TypeImage)
	if got != "" {
		t.Errorf("wrong-type profile got %q, want empty", got)
	}
}

type bogusProfile struct{ typeName string }

func (b *bogusProfile) Type() string    { return b.typeName }
func (b *bogusProfile) Version() string { return "x" }
func (b *bogusProfile) Name() string    { return "x" }
