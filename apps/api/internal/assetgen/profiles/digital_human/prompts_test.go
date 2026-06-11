package digital_human_test

import (
	"strings"
	"testing"

	"github.com/opc/api/internal/assetgen"
	"github.com/opc/api/internal/assetgen/profiles/digital_human"
)

func TestBuildDigitalHumanImage(t *testing.T) {
	t.Parallel()

	p := digital_human.DefaultInstance()
	got := digital_human.BuildPrompt(p, "演播室", "蓝色西装", assetgen.TypeImage)

	wantContains := []string{
		"莉娜", "28", "female", "东亚", "鹅蛋脸",
		"rgb(245,228,210)", "黑色长发披肩", "温柔知性, 普通话",
		"演播室", "蓝色西装",
		"--ratio 9:16",
	}
	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Errorf("BuildPrompt(image) missing %q", s)
		}
	}
}
