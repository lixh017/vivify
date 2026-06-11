package digital_human_test

import (
	"testing"

	"github.com/opc/api/internal/assetgen/profiles/digital_human"
)

func TestDigitalHumanProfileRoundTrip(t *testing.T) {
	t.Parallel()

	p := digital_human.DefaultInstance()
	if p == nil {
		t.Fatal("DefaultInstance returned nil")
	}
	if p.Type_ != "digital_human" {
		t.Errorf("Type_ = %q, want digital_human", p.Type_)
	}
	if p.Version_ != "lina_v1" {
		t.Errorf("Version_ = %q, want lina_v1", p.Version_)
	}
	if p.Age != 28 {
		t.Errorf("Age = %d, want 28", p.Age)
	}
}
