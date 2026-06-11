package costume_test

import (
	"testing"

	"github.com/opc/api/internal/assetgen/profiles/costume"
)

func TestCostumeProfileRoundTrip(t *testing.T) {
	t.Parallel()

	p := costume.DefaultInstance()
	if p == nil {
		t.Fatal("DefaultInstance returned nil")
	}
	if p.Type_ != "costume" {
		t.Errorf("Type_ = %q, want costume", p.Type_)
	}
	if p.Version_ != "xianyun_v1" {
		t.Errorf("Version_ = %q, want xianyun_v1", p.Version_)
	}
	if p.Era != "唐代" {
		t.Errorf("Era = %q, want 唐代", p.Era)
	}
	if len(p.CostumeLayer) < 2 {
		t.Errorf("len(CostumeLayer) = %d, want >= 2", len(p.CostumeLayer))
	}
}
