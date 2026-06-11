package assetgen_test

import (
	"testing"

	"github.com/opc/api/internal/assetgen"
)

// TestBuildPromptUnknownType confirms the dispatcher returns ""
// for unknown IP types so callers can detect and fallback.
func TestBuildPromptUnknownType(t *testing.T) {
	t.Parallel()

	bogus := &bogusProfile{typeName: "unicorn"}
	got := assetgen.BuildPrompt(bogus, "scene", "outfit", assetgen.TypeImage)
	if got != "" {
		t.Errorf("BuildPrompt(unicorn) = %q, want empty string", got)
	}
}
