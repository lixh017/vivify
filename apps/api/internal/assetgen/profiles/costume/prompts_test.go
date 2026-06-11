package costume_test

import (
	"strings"
	"testing"

	"github.com/opc/api/internal/assetgen"
	"github.com/opc/api/internal/assetgen/profiles/costume"
)

func TestBuildCostumeImage(t *testing.T) {
	t.Parallel()

	p := costume.DefaultInstance()
	got := costume.BuildPrompt(p, "竹林", "红色披风", assetgen.TypeImage)

	wantContains := []string{
		"纤云", "唐代", "侠女", "襦裙", "长剑",
		"竹林", "红色披风",
		"无穿越",
		"--ratio 9:16",
	}
	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Errorf("BuildPrompt(image) missing %q", s)
		}
	}
}
