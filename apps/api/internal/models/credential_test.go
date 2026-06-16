package models

import (
	"strings"
	"testing"
)

func TestCredential_Validate_AcceptsAnthropicProtocol(t *testing.T) {
	c := Credential{
		Name:      "primary",
		Provider:  "anthropic",
		Protocol:  "anthropic",
		ModelName: "claude-sonnet-4-5",
		Scope:     "all",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if c.Protocol != "anthropic" {
		t.Fatalf("want normalized protocol=anthropic, got %q", c.Protocol)
	}
}

func TestCredential_Validate_RejectsBadProtocol(t *testing.T) {
	c := Credential{
		Name:      "primary",
		Provider:  "anthropic",
		Protocol:  "gopher",
		ModelName: "claude-sonnet-4-5",
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("want error for protocol=gopher, got nil")
	}
	if !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("want error mentioning protocol, got %v", err)
	}
}

func TestCredential_Validate_RequiresModelForNewProtocols(t *testing.T) {
	c := Credential{
		Name:     "primary",
		Provider: "anthropic",
		Protocol: "anthropic",
		// ModelName missing
	}
	if err := c.Validate(); err == nil {
		t.Fatal("want error for missing model_name, got nil")
	}
}

func TestCredential_Validate_NormalizesProtocolCase(t *testing.T) {
	c := Credential{
		Name:      "primary",
		Provider:  "anthropic",
		Protocol:  "ANTHROPIC",
		ModelName: "claude-sonnet-4-5",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("want nil for uppercase protocol, got %v", err)
	}
	if c.Protocol != "anthropic" {
		t.Fatalf("want normalized protocol=anthropic, got %q", c.Protocol)
	}
}

func TestProviderIsAllowed_IncludesAnthropicAndOpenAI(t *testing.T) {
	for _, p := range []string{"anthropic", "openai", "Anthropic", "OPENAI"} {
		if !ProviderIsAllowed(p) {
			t.Errorf("want %q to be allowed", p)
		}
	}
}
