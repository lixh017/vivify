package assetgen

// Round2Public rounds a float to 2 decimal places. Exported for
// use by sub-package check functions (e.g. anthropomorphic,
// digital_human) to match the JSONL ledger format from the
// original roundtrip-volcengine.mjs script.
func Round2Public(f float64) float64 {
	return round2(f)
}
