package assetgen

import "fmt"

// registry is populated by sub-package init() calls. Map keyed by
// IP type discriminator (Profile.Type() return value). Read-only
// after init time.
var registry = map[string]ProfileEntry{}

// Register adds an entry to the registry. Panics on duplicate
// type — this is a fail-fast at startup, not a runtime check,
// because duplicate registration means a build error, not a
// runtime condition.
func Register(typeName string, entry ProfileEntry) {
	if _, exists := registry[typeName]; exists {
		panic(fmt.Sprintf("assetgen: duplicate Register for type %q", typeName))
	}
	if entry.Schema == nil {
		panic(fmt.Sprintf("assetgen: Register for %q has nil Schema", typeName))
	}
	if entry.Check == nil {
		panic(fmt.Sprintf("assetgen: Register for %q has nil Check", typeName))
	}
	if entry.BuildPrompt == nil {
		panic(fmt.Sprintf("assetgen: Register for %q has nil BuildPrompt", typeName))
	}
	registry[typeName] = entry
}

// LoadProfile returns the registered Profile schema for the
// given IP type. Empty typeName defaults to "anthropomorphic"
// for back-compat with Phase 1 callers that used the empty
// string as "the only profile we ship".
//
// Returns an error for unknown types so callers can fail loudly
// rather than silently fall back.
func LoadProfile(typeName string) (Profile, error) {
	if typeName == "" {
		typeName = "anthropomorphic"
	}
	entry, ok := registry[typeName]
	if !ok {
		return nil, fmt.Errorf("assetgen: unknown IP type %q (registered: %v)", typeName, registeredTypes())
	}
	return entry.Schema, nil
}

// MustLoadProfile is LoadProfile that panics on error. Use only
// in places where a missing profile is a programmer error (e.g.
// CLI main wiring).
func MustLoadProfile(typeName string) Profile {
	p, err := LoadProfile(typeName)
	if err != nil {
		panic(err)
	}
	return p
}

// registeredTypes returns a sorted-ish list of registered type
// names, used only for error messages. Order is non-deterministic
// (map iteration) but M1 has 4 types max so readability is fine.
func registeredTypes() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}
