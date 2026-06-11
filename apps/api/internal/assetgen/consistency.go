package assetgen

import (
	"fmt"
	"math"
)

// ConsistencyResult is the output of ConsistencyCheck. Score is
// in [0, 1]; Fails lists the human-readable names of the anchors
// the prompt is missing, in the order they were checked (most
// brand-critical first).
type ConsistencyResult struct {
	Score float64
	Fails []string
}

// ConsistencyCheck scores a prompt against the profile's brand
// anchors by dispatching to the type-specific check function
// registered for the profile's IP type.
//
// The dispatcher is fail-safe: nil profile, unknown type, and
// panicking check functions all return Score=0 with an explicit
// fail message — they never crash the caller.
func ConsistencyCheck(p Profile, prompt string) ConsistencyResult {
	return safeCheck(p, prompt)
}

// safeCheck is the actual dispatcher with the recover() guard.
func safeCheck(p Profile, prompt string) (result ConsistencyResult) {
	if p == nil {
		return ConsistencyResult{Score: 0, Fails: []string{"nil profile"}}
	}
	entry, ok := registry[p.Type()]
	if !ok {
		return ConsistencyResult{
			Score: 0,
			Fails: []string{fmt.Sprintf("unknown profile type: %s", p.Type())},
		}
	}
	defer func() {
		if r := recover(); r != nil {
			result = ConsistencyResult{
				Score: 0,
				Fails: []string{fmt.Sprintf("check panic: %v", r)},
			}
		}
	}()
	return entry.Check(p, prompt)
}

// round2 rounds a float to 2 decimal places, matching the JSONL
// ledger format the original mjs used. Private; sub-packages
// use Round2Public instead.
func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
