package costume_test

import (
	"testing"

	"github.com/opc/api/internal/assetgen/profiles/costume"
)

func TestCheckCostumePerfect(t *testing.T) {
	t.Parallel()

	p := costume.DefaultInstance()
	prompt := "纤云, 唐代, 侠女, 襦裙, 披帛, 绣花鞋, 长剑, 酒壶, 书卷, 朱红#C73E1D, 鹅黄#F5DEB3, 黛绿#2F4F4F, 无穿越, 色调统一"
	got := costume.Check(p, prompt)
	if got.Score < 0.99 {
		t.Errorf("perfect prompt scored %v, want >= 0.99", got.Score)
	}
}

func TestCheckCostumePartial(t *testing.T) {
	t.Parallel()

	p := costume.DefaultInstance()
	prompt := "纤云 唐代"
	got := costume.Check(p, prompt)
	if got.Score != 0.35 {
		t.Errorf("partial scored %v, want 0.35", got.Score)
	}
	if len(got.Fails) != 5 {
		t.Errorf("len(Fails) = %d, want 5", len(got.Fails))
	}
}

func TestCheckCostumeZero(t *testing.T) {
	t.Parallel()

	p := costume.DefaultInstance()
	got := costume.Check(p, "完全无关的文本")
	if got.Score > 0.01 {
		t.Errorf("zero scored %v, want 0.0", got.Score)
	}
}

func TestCheckCostumeNil(t *testing.T) {
	t.Parallel()

	got := costume.Check(nil, "anything")
	if got.Score != 0 {
		t.Errorf("nil scored %v, want 0", got.Score)
	}
}

func TestCheckCostumeNoAnachronism(t *testing.T) {
	t.Parallel()

	p := costume.DefaultInstance()
	prompt := "纤云, 唐代, 侠女, 无穿越"
	got := costume.Check(p, prompt)
	if got.Score != 0.60 {
		t.Errorf("anachronism prompt scored %v, want 0.60", got.Score)
	}
}
