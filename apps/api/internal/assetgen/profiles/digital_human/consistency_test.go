package digital_human_test

import (
	"testing"

	"github.com/opc/api/internal/assetgen/profiles/digital_human"
)

func TestCheckDigitalHumanPerfect(t *testing.T) {
	t.Parallel()

	p := digital_human.DefaultInstance()
	prompt := "莉娜, 28 岁, female, 东亚, 鹅蛋脸, 肤色 rgb(245,228,210), 黑色长发披肩, 声线 温柔知性, 普通话, 身穿 蓝色西装, 演播室灯光, 写实质感, 镜头特写, 表情自然, 无恐怖谷"
	got := digital_human.Check(p, prompt)
	if got.Score < 0.99 {
		t.Errorf("perfect prompt scored %v, want >= 0.99", got.Score)
	}
}

func TestCheckDigitalHumanPartial(t *testing.T) {
	t.Parallel()

	p := digital_human.DefaultInstance()
	prompt := "莉娜 28 岁 female"
	got := digital_human.Check(p, prompt)
	if got.Score != 0.35 {
		t.Errorf("partial scored %v, want 0.35", got.Score)
	}
	if len(got.Fails) != 5 {
		t.Errorf("len(Fails) = %d, want 5", len(got.Fails))
	}
}

func TestCheckDigitalHumanZero(t *testing.T) {
	t.Parallel()

	p := digital_human.DefaultInstance()
	got := digital_human.Check(p, "完全无关的文本")
	if got.Score > 0.01 {
		t.Errorf("zero scored %v, want 0.0", got.Score)
	}
}

func TestCheckDigitalHumanNil(t *testing.T) {
	t.Parallel()

	got := digital_human.Check(nil, "anything")
	if got.Score != 0 {
		t.Errorf("nil scored %v, want 0", got.Score)
	}
}

func TestCheckDigitalHumanSkinToneWeight(t *testing.T) {
	t.Parallel()

	p := digital_human.DefaultInstance()
	prompt := "rgb(245,228,210)"
	got := digital_human.Check(p, prompt)
	if got.Score != 0.15 {
		t.Errorf("skin-tone-only scored %v, want 0.15", got.Score)
	}
}
