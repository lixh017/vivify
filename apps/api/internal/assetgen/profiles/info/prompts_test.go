package info_test

import (
	"strings"
	"testing"

	"github.com/opc/api/internal/assetgen"
	"github.com/opc/api/internal/assetgen/profiles/info"
)

func TestBuildInfoImage(t *testing.T) {
	t.Parallel()

	p := info.DefaultInstance()
	got := info.BuildPrompt(p, "演播室", "黑体加粗", assetgen.TypeImage)

	wantContains := []string{
		"OPC 每日资讯", "演播室双主播", "主播阿橙",
		"黑体加粗, 暖色字幕", "宝蓝#1F5FA8", "rgb(245,240,225)",
		"演播室", "无 AI 播报感",
		"--ratio 9:16",
	}
	for _, s := range wantContains {
		if !strings.Contains(got, s) {
			t.Errorf("BuildPrompt(image) missing %q", s)
		}
	}
}
