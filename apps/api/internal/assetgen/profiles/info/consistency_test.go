package info_test

import (
	"testing"

	"github.com/opc/api/internal/assetgen/profiles/info"
)

func TestCheckInfoPerfect(t *testing.T) {
	t.Parallel()

	p := info.DefaultInstance()
	prompt := "OPC 每日资讯, 演播室双主播, 主播阿橙, 黑体加粗, 暖色字幕, 宝蓝#1F5FA8, rgb(245,240,225), 无 AI 播报感"
	got := info.Check(p, prompt)
	if got.Score < 0.99 {
		t.Errorf("perfect prompt scored %v, want >= 0.99", got.Score)
	}
}

func TestCheckInfoPartial(t *testing.T) {
	t.Parallel()

	p := info.DefaultInstance()
	// Prompt includes only HostPersona (主播阿橙) and NewsroomStyle
	// (演播室双主播). Spec §7.2.4 weights are 0.20 + 0.20 = 0.40 with
	// 4 fails (font_tone / accent_color / no_ai_broadcast /
	// background_color missing). Note: the OPC 每日资讯 name token
	// is in the prompt but is NOT a consistency anchor per §7.2.4
	// (6 anchors only).
	prompt := "OPC 每日资讯, 演播室双主播, 主播阿橙"
	got := info.Check(p, prompt)
	if got.Score != 0.40 {
		t.Errorf("partial scored %v, want 0.40", got.Score)
	}
	if len(got.Fails) != 4 {
		t.Errorf("len(Fails) = %d, want 4", len(got.Fails))
	}
}

func TestCheckInfoZero(t *testing.T) {
	t.Parallel()

	p := info.DefaultInstance()
	got := info.Check(p, "完全无关的文本")
	if got.Score > 0.01 {
		t.Errorf("zero scored %v, want 0.0", got.Score)
	}
}

func TestCheckInfoNil(t *testing.T) {
	t.Parallel()

	got := info.Check(nil, "anything")
	if got.Score != 0 {
		t.Errorf("nil scored %v, want 0", got.Score)
	}
}

func TestCheckInfoNoAIBroadcast(t *testing.T) {
	t.Parallel()

	p := info.DefaultInstance()
	prompt := "OPC 每日资讯, 演播室双主播, 主播阿橙, 黑体加粗, 暖色字幕, 宝蓝#1F5FA8, rgb(245,240,225)"
	got := info.Check(p, prompt)
	if got.Score != 0.80 {
		t.Errorf("no-ai-broadcast prompt scored %v, want 0.80", got.Score)
	}
}
