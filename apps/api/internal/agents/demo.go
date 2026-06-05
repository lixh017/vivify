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

// IsDemoMode reports whether the Claude agent should be served from
// canned data — either because no API key was configured at
// construction time, or because the caller explicitly asked
// (e.g. ?demo=true on a request). The handler combines the two via
// a single boolean to keep the call sites short.
func (c *Claude) IsDemoMode(forceDemo bool) bool {
	return forceDemo || !c.keyConfigured
}

// KeyConfigured reports whether the agent was constructed with a
// non-empty API key. The handler uses this to decide whether to
// fall back to demo mode automatically.
func (c *Claude) KeyConfigured() bool {
	return c.keyConfigured
}

// DemoResponse returns the pre-canned response for the given
// operation. It never errors — the data is compiled into the
// binary, not loaded from disk or the network — so the handler can
// treat the (string, error) shape the same as Complete().
//
// The seed parameter lets the caller vary which canned response is
// returned across multiple calls; pass 0 if you do not care. For
// the topics op there is only one canned array, so the seed is
// ignored.
func (c *Claude) DemoResponse(op DemoOperation, seed int) (string, error) {
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
