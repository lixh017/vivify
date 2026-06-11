package anthropomorphic_test

import (
	"testing"

	"github.com/opc/api/internal/assetgen/profiles/anthropomorphic"
)

func TestCheckAnthropomorphicPerfect(t *testing.T) {
	t.Parallel()

	p := anthropomorphic.DefaultInstance()
	prompt := "峰哥, 成年熊猫, 头身比 1:1.2 圆胖身材, 面部白 rgb(245,240,225), 眼周黑 rgb(26,26,26), 眼神半阖带笑意, 不直视镜头, 身穿朱红汉服, 站在竹林小院, 暖光氛围, 国潮 + 色彩靓丽(朱红#C73E1D/暖橙#E89B45/翠绿#3B8C5A 撞色, 非暗色调、非水墨), 不露爪, 不攻击性姿势, 无文字, 不要 AI 生成感"
	got := anthropomorphic.Check(p, prompt)
	if got.Score < 0.99 {
		t.Errorf("perfect prompt scored %v, want >= 0.99", got.Score)
	}
}

func TestCheckAnthropomorphicPartial(t *testing.T) {
	t.Parallel()

	p := anthropomorphic.DefaultInstance()
	prompt := "峰哥 在竹林"
	got := anthropomorphic.Check(p, prompt)
	if got.Score != 0.20 {
		t.Errorf("partial prompt scored %v, want 0.20", got.Score)
	}
	if len(got.Fails) != 6 {
		t.Errorf("len(Fails) = %d, want 6 (Fails: %v)", len(got.Fails), got.Fails)
	}
}

func TestCheckAnthropomorphicZero(t *testing.T) {
	t.Parallel()

	p := anthropomorphic.DefaultInstance()
	got := anthropomorphic.Check(p, "完全无关的文本")
	if got.Score > 0.01 {
		t.Errorf("zero prompt scored %v, want 0.0", got.Score)
	}
	if len(got.Fails) != 7 {
		t.Errorf("len(Fails) = %d, want 7", len(got.Fails))
	}
}

func TestCheckAnthropomorphicTheme(t *testing.T) {
	t.Parallel()

	p := anthropomorphic.DefaultInstance()
	prompt := "峰哥, 成年熊猫, rgb(245,240,225), rgb(26,26,26), 国潮"
	got := anthropomorphic.Check(p, prompt)
	if got.Score != 0.90 {
		t.Errorf("theme prompt scored %v, want 0.90", got.Score)
	}
	if len(got.Fails) != 2 {
		t.Errorf("len(Fails) = %d, want 2 (anti-claws + anti-AI)", len(got.Fails))
	}
}

func TestCheckAnthropomorphicNil(t *testing.T) {
	t.Parallel()

	got := anthropomorphic.Check(nil, "anything")
	if got.Score != 0 {
		t.Errorf("nil profile scored %v, want 0", got.Score)
	}
}
