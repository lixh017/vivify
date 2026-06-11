package agents

// DemoOperation identifies which canned response the demo mode
// should return. Defined as a typed string so callers cannot pass
// arbitrary values and the compiler catches typos.
type DemoOperation string

const (
	// DemoOpTopics returns a pre-canned 5-topic JSON array
	// matching the shape /ai/topics expects.
	DemoOpTopics DemoOperation = "topics"
	// DemoOpHumanize returns a pre-canned humanized script
	// matching the plain-text shape /ai/humanize expects.
	DemoOpHumanize DemoOperation = "humanize"
	// DemoOpPostmortem returns a pre-canned postmortem JSON
	// object matching the shape /ai/postmortem expects.
	DemoOpPostmortem DemoOperation = "postmortem"
	// DemoOpScore returns a pre-canned QualityScoreResponse for
	// the panda-IP sample script — used by /ai/score demo mode.
	DemoOpScore DemoOperation = "score"
	// DemoOpPlatformAdapt returns a pre-canned set of three
	// platform adaptations (抖音/哔哩哔哩/小红书) for the panda
	// IP, used by /ai/platform-adapt demo mode.
	DemoOpPlatformAdapt DemoOperation = "platform_adapt"
	// DemoOpDeconstruct returns a pre-canned OpusClip-style video
	// deconstruction (hook + structure + cta + emotional_arc +
	// reusable_patterns) for the panda-IP sample video, used by
	// /ai/deconstruct demo mode.
	DemoOpDeconstruct DemoOperation = "deconstruct"
	// DemoOpViralFormula returns a pre-canned reusable formula
	// extracted from the panda-IP sample video, used by
	// /ai/viral-formula demo mode.
	DemoOpViralFormula DemoOperation = "viral_formula"
)

// IsDemoMode / KeyConfigured were *Claude method shims that lived
// here before the Phase-4 MiniMax migration. The new world has
// no legacy Claude shim — handlers use MiniMax.Available() + the
// isDemoRequest query param directly — so these helpers have been
// removed. The shouldUseDemo methods on each handler replicate
// the combined check (forceDemo || !Available) in one line.

// DemoResponse returns the pre-canned response for the given
// operation. It never errors — the data is compiled into the
// binary, not loaded from disk or the network — so the handler can
// treat the (string, error) shape the same as Complete().
//
// The seed parameter lets the caller vary which canned response is
// returned across multiple calls; pass 0 if you do not care. For
// the topics op there is only one canned array, so the seed is
// ignored.
//
// Package-level function (not a method) so handlers can call it
// without the legacy Claude shim. The (c *Claude) method form
// below is preserved as a thin shim for any leftover test code
// that still constructs a Claude for demo data only.
func DemoResponse(op DemoOperation, seed int) (string, error) {
	switch op {
	case DemoOpTopics:
		return DemoTopicsJSON, nil
	case DemoOpHumanize:
		idx := pickDemoIndex(seed, len(DemoHumanizedScripts))
		return DemoHumanizedScripts[idx], nil
	case DemoOpPostmortem:
		idx := pickDemoIndex(seed, len(DemoPostmortems))
		return DemoPostmortems[idx], nil
	case DemoOpScore:
		idx := pickDemoIndex(seed, len(DemoQualityScores))
		return DemoQualityScores[idx], nil
	case DemoOpPlatformAdapt:
		idx := pickDemoIndex(seed, len(DemoPlatformAdapts))
		return DemoPlatformAdapts[idx], nil
	case DemoOpDeconstruct:
		idx := pickDemoIndex(seed, len(DemoDeconstructs))
		return DemoDeconstructs[idx], nil
	case DemoOpViralFormula:
		idx := pickDemoIndex(seed, len(DemoViralFormulas))
		return DemoViralFormulas[idx], nil
	default:
		// Unknown op — fall back to topics so the demo still has
		// *something* useful to show. We do not error here because
		// the caller has no good recovery path; an unknown op is a
		// programmer error caught by code review, not a runtime
		// failure mode.
		return DemoTopicsJSON, nil
	}
}
