package info_test

import (
	"testing"

	"github.com/opc/api/internal/assetgen/profiles/info"
)

func TestInfoProfileRoundTrip(t *testing.T) {
	t.Parallel()

	p := info.DefaultInstance()
	if p == nil {
		t.Fatal("DefaultInstance returned nil")
	}
	if p.Type_ != "info" {
		t.Errorf("Type_ = %q, want info", p.Type_)
	}
	if p.Version_ != "opc_daily_v1" {
		t.Errorf("Version_ = %q, want opc_daily_v1", p.Version_)
	}
	if p.NewsroomStyle != "演播室双主播" {
		t.Errorf("NewsroomStyle = %q, want 演播室双主播", p.NewsroomStyle)
	}
}
