package anthropomorphic_test

import (
	"encoding/json"
	"testing"

	"github.com/opc/api/internal/assetgen/profiles/anthropomorphic"
)

func TestProfileRoundTrip(t *testing.T) {
	t.Parallel()

	p := anthropomorphic.DefaultInstance()
	if p == nil {
		t.Fatal("DefaultInstance returned nil")
	}
	if p.Type_ != "anthropomorphic" {
		t.Errorf("Type_ = %q, want anthropomorphic", p.Type_)
	}
	if p.Version_ != "fengge_v1" {
		t.Errorf("Version_ = %q, want fengge_v1", p.Version_)
	}
	if p.Name_ != "峰哥" {
		t.Errorf("Name_ = %q, want 峰哥", p.Name_)
	}
	if len(p.Palette) < 3 {
		t.Errorf("len(Palette) = %d, want >= 3", len(p.Palette))
	}

	blob, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var got anthropomorphic.Profile
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.Name_ != p.Name_ {
		t.Errorf("round-trip Name_ = %q, want %q", got.Name_, p.Name_)
	}
	if len(got.Palette) != len(p.Palette) {
		t.Errorf("round-trip Palette length = %d, want %d", len(got.Palette), len(p.Palette))
	}
}

func TestProfileImplementsAssetgenProfile(t *testing.T) {
	t.Parallel()

	var _ assetgenAlias = (*anthropomorphic.Profile)(nil)
}

type assetgenAlias interface {
	Type() string
	Version() string
	Name() string
}
