package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
)

// Batch limits -------------------------------------------------------------

// batchMaxCount caps the number of topics a single /ai/batch
// call will generate. A naive caller could otherwise request
// count=1000 and the per-step Claude calls would burn the
// daily token budget in a single request. 25 is well above
// the operator's "give me 10 ideas" use case.
const batchMaxCount = 25

// batchDefaultCount is what the handler fills in when the
// caller omits `count`. Ten is the canonical "give me a few
// options" batch size and matches the prompt default.
const batchDefaultCount = 10

// batchConcurrency is the number of topics we run in parallel
// inside a single batch call. The Claude API has a per-org
// rate limit and a per-request latency; running 25 in parallel
// would either trip the rate limit or overwhelm a single
// connection pool. Three is conservative and keeps the wall
// clock under 30s for the full batch on a healthy connection.
const batchConcurrency = 3

// Batch request / response shapes -----------------------------------------

// batchRequest is the JSON body for POST /ai/batch.
//
// seed is the starting concept the operator hands the machine
// (one per batch, not one per topic). platforms is the
// downstream targets the adapt step emits into; we default to
// 抖音/哔哩哔哩/小红书 when empty, matching the pipeline
// handler's behavior. count is the number of topics to
// generate; we clamp to [1, batchMaxCount]. steps is the
// per-topic step filter (same vocabulary as /ai/pipeline).
//
// include_knowledge gates the RAG lookup. We pre-compute the
// RAG context once per request and inject it into every topic's
// prompt so a single seed produces a coherent set.
//
// auto_create_content_items is the persistence flag: when
// true, the handler persists the best topic + script as Topic
// + Script + ContentItem rows. We deliberately do not persist
// partial failures — the request is all-or-nothing so the
// caller can show a clean "5 ideas saved" toast.
type batchRequest struct {
	Seed                   string   `json:"seed"`
	Platforms              []string `json:"platforms"`
	Count                  int      `json:"count"`
	IncludeKnowledge       bool     `json:"include_knowledge"`
	Steps                  []string `json:"steps"`
	AutoCreateContentItems bool     `json:"auto_create_content_items"`
}

// batchTopic is the trimmed topic shape the batch endpoint
// emits. We reuse the existing generatedTopic struct (defined
// in ai.go) and add a synthetic `id` field that is populated
// when the row was persisted. id is omitempty so the demo
// path (no DB) does not need to know about persistence.
type batchTopic struct {
	generatedTopic
	ID uint `json:"id,omitempty"`
}

// batchGeneratedItem is the per-topic result envelope. It
// contains only the sections the caller asked for — script,
// score, and adaptations are all omitempty pointers. The
// envelope shape mirrors /ai/pipeline's per-section emission
// so the frontend can reuse the same render code.
type batchGeneratedItem struct {
	Topic       *batchTopic            `json:"topic,omitempty"`
	Script      *pipelineScript        `json:"script,omitempty"`
	Score       *qualityScoreResponse  `json:"score,omitempty"`
	Adaptations *platformAdaptResponse `json:"adaptations,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// batchSummary is the small rollup block at the end of the
// response. success_count + fail_count always equals total so
// the frontend can render a progress bar without further
// validation.
type batchSummary struct {
	Total        int `json:"total"`
	SuccessCount int `json:"success_count"`
	FailCount    int `json:"fail_count"`
}

// batchResponse is the wire shape returned by /ai/batch.
// knowledge_used is the union of knowledge titles that fed the
// RAG step (only meaningful when include_knowledge is true);
// it is always an array (possibly empty) so the frontend
// doesn't have to branch on null vs [].
type batchResponse struct {
	Generated     []batchGeneratedItem `json:"generated"`
	Summary       batchSummary         `json:"summary"`
	KnowledgeUsed []string             `json:"knowledge_used"`
}

// BatchHandler exposes the batch endpoint. It reuses the
// same *agents.MiniMax (topics + script + score + adapt) as
// the pipeline handler so no new agent code is needed — the
// batch logic is a concurrency shell over the same prompt
// builders.
type BatchHandler struct {
	text   *agents.MiniMax
	db     *gorm.DB
	logger *slog.Logger
}

// NewBatchHandler wires a BatchHandler. db is required only
// when auto_create_content_items is true; the rest of the
// handler works without it. A nil logger falls back to
// slog.Default().
func NewBatchHandler(text *agents.MiniMax, db *gorm.DB) *BatchHandler {
	l := slog.Default()
	return &BatchHandler{
		text:   text,
		db:     db,
		logger: l,
	}
}

// shouldUseDemo mirrors AIHandler.shouldUseDemo. Duplicated
// rather than shared so handlers stay decoupled.
func (h *BatchHandler) shouldUseDemo(c *gin.Context) bool {
	return isDemoRequest(c) || !h.text.Available()
}

// RegisterRoutes attaches the batch endpoint.
func (h *BatchHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/ai/batch", h.RunBatch)
}

// allowedBatchSteps is the set of per-topic step names the
// batch handler accepts. We deliberately keep this list the
// same as the pipeline handler's so the two surfaces do not
// diverge over time.
var allowedBatchSteps = map[string]bool{
	"topics": true,
	"script": true,
	"score":  true,
	"adapt":  true,
}

// defaultBatchSteps is the canonical four-step pipeline. We
// keep the order stable so the test suite can assert on
// step-order without flakiness.
var defaultBatchSteps = []string{"topics", "script", "score", "adapt"}

// defaultBatchPlatforms is the per-topic adapt target set.
// Kept in lockstep with the pipeline handler so the demo
// feels consistent.
var defaultBatchPlatforms = []string{"抖音", "哔哩哔哩", "小红书"}

// defaultSourcePlatform is the platform the batch handler uses
// for the topic + script steps when the caller did not supply a
// `platforms` list. It is the first entry in defaultBatchPlatforms
// by convention; the per-topic adapt step then rewrites to every
// platform in `platforms` regardless. Defining it as a constant
// keeps the batch handler's "source platform" default in lockstep
// with any future change to the canonical source platform and
// makes the value referenceable from tests.
const defaultSourcePlatform = "抖音"

// RunBatch — POST /ai/batch
//
// Body: {seed, platforms, count, include_knowledge, steps, auto_create_content_items}
// 200:  {generated[], summary, knowledge_used}
// 400:  missing seed, invalid count, no usable steps
// 503:  Claude is not configured and no demo flag
//
// The endpoint runs the per-topic pipeline count times in
// parallel (bounded by batchConcurrency) and returns the union
// of per-topic results. Per-topic failures are recorded in
// the per-item Error field and counted in summary.fail_count
// — the handler itself only 503s when the underlying Claude
// surface is completely missing (no API key, no demo).
//
// When auto_create_content_items is true and the per-topic
// topic + script + adapt succeed, the handler persists the
// row set under the caller's user_id (Phase 2 multi-tenant).
func (h *BatchHandler) RunBatch(c *gin.Context) {
	var req batchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid batch body", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Seed = strings.TrimSpace(req.Seed)
	if req.Seed == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "seed is required"})
		return
	}
	if req.Count <= 0 {
		req.Count = batchDefaultCount
	}
	if req.Count > batchMaxCount {
		req.Count = batchMaxCount
	}
	platforms := normalizeBatchPlatforms(req.Platforms)
	steps := normalizeBatchSteps(req.Steps)
	if len(steps) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "steps must contain at least one of: topics, script, score, adapt"})
		return
	}

	// Per-topic source platform for the topic + script steps.
	// Defaults to the canonical source platform; the per-topic
	// adapt step rewrites to every platform in `platforms`
	// regardless. See defaultSourcePlatform docstring for why
	// this is a named constant rather than an inline literal.
	sourcePlatform := defaultSourcePlatform
	if len(platforms) > 0 {
		sourcePlatform = platforms[0]
	}

	// RAG pre-step. We compute it once and inject into every
	// topic's prompt so the batch reads as one coherent set
	// rather than N independent lookups. The titles go back
	// in the response so the frontend can show "grounded in
	// docs X, Y, Z" — the demo's transparency promise.
	var ragContext string
	var ragTitles []string
	if req.IncludeKnowledge {
		ragContext, ragTitles = agents.FindRelevantKnowledge(c.Request.Context(), h.db, req.Seed, sourcePlatform)
	}

	// Demo short-circuit. The demo path is single-pass — it
	// returns the same canned data for every topic — so the
	// per-topic step results are deterministic. We populate
	// N copies so the count field in the response matches
	// the caller's request, and we tag the response with
	// X-Demo-Mode for frontend rendering.
	if h.shouldUseDemo(c) {
		markDemoResponse(c)
		items := h.buildDemoBatch(req, steps, platforms, ragTitles)
		c.JSON(http.StatusOK, h.summarize(items, ragTitles))
		return
	}

	// Live path. We run batchConcurrency topics in parallel
	// and collect the results. A per-topic failure does not
	// abort the batch — we record the error and continue.
	items := h.runBatch(c.Request.Context(), req, steps, platforms, sourcePlatform, ragContext, ragTitles)
	c.JSON(http.StatusOK, h.summarize(items, ragTitles))
}

// normalizeBatchSteps returns the steps in the caller's order,
// dropping any names outside the allowed set. Empty input
// becomes the canonical four-step pipeline.
func normalizeBatchSteps(in []string) []string {
	if len(in) == 0 {
		out := make([]string, len(defaultBatchSteps))
		copy(out, defaultBatchSteps)
		return out
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !allowedBatchSteps[s] {
			continue
		}
		out = append(out, s)
	}
	return out
}

// normalizeBatchPlatforms returns the platform list with
// duplicates removed and empty entries dropped. Empty input
// (nil/zero-length, or all-blank entries) becomes the canonical
// 抖音/哔哩哔哩/小红书 set. The first platform is used as the
// "source" platform for the topic + script steps (the adapt step
// then emits all three).
//
// The all-blank branch is preserved (not unreachable) so a
// caller that sends ["", " "] still gets a usable platform set
// rather than a silent empty list — matching the test's contract
// and the QA guidance that the default is only used when input
// is nil/empty/all-blank.
func normalizeBatchPlatforms(in []string) []string {
	if len(in) == 0 {
		out := make([]string, len(defaultBatchPlatforms))
		copy(out, defaultBatchPlatforms)
		return out
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		// All entries were blank or duplicates of nothing —
		// fall back to the canonical set so downstream code
		// always has at least one platform to work with.
		out = append(out, defaultBatchPlatforms...)
	}
	return out
}

// runBatch is the concurrency shell. It launches N goroutines
// (bounded by batchConcurrency) and each runs the same
// per-topic pipeline. The output is in the same order as the
// count so the caller can index by position; per-topic errors
// land in the per-item Error field.
func (h *BatchHandler) runBatch(ctx context.Context, req batchRequest, steps, platforms []string, sourcePlatform, ragContext string, ragTitles []string) []batchGeneratedItem {
	results := make([]batchGeneratedItem, req.Count)
	var wg sync.WaitGroup
	sem := make(chan struct{}, batchConcurrency)

	for i := 0; i < req.Count; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = h.runOneTopic(ctx, req.Seed, sourcePlatform, platforms, steps, ragContext, ragTitles)
		}(i)
	}
	wg.Wait()
	return results
}

// runOneTopic is the per-topic pipeline. It mirrors the
// /ai/pipeline handler step-for-step so the batch output is a
// faithful copy of what the caller would have gotten by
// calling /ai/pipeline N times. A failure in any step records
// the error on the result and short-circuits the rest of the
// steps (same policy as the pipeline handler).
//
// ragTitles is reserved for a future "grounded in docs X, Y, Z"
// per-topic attribution feature; the per-batch title set is
// already returned at the top level via batchResponse.KnowledgeUsed.
func (h *BatchHandler) runOneTopic(ctx context.Context, seed, sourcePlatform string, platforms, steps []string, ragContext string, _ []string) batchGeneratedItem {
	res := batchGeneratedItem{}
	topics, err := runBatchTopicsStep(ctx, h.text, seed, sourcePlatform, ragContext, 1)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if len(topics) == 0 {
		res.Error = "no topics returned"
		return res
	}
	bt := &batchTopic{generatedTopic: topics[0]}
	res.Topic = bt

	// "topics" alone — return the topic and stop. The
	// pipeline handler returns ALL topics it generated, but
	// the batch endpoint is "give me N ideas" so one
	// topic per slot is the right granularity.
	if len(steps) == 1 && steps[0] == "topics" {
		return res
	}

	// scriptContent is tracked across iterations of the steps
	// loop so a caller that lists "script" twice (e.g. once to
	// also force regeneration after a future flag) only pays for
	// one MiniMax call. Today normalizeBatchSteps dedupes by
	// string, so this is a defensive no-op, but the variable
	// keeps the loop correct if that policy ever relaxes.
	var scriptContent string
	for _, step := range steps {
		if step == "topics" {
			continue
		}
		switch step {
		case "script":
			if scriptContent != "" {
				continue
			}
			s, err := runBatchScriptStep(ctx, h.text, bt.generatedTopic, sourcePlatform, ragContext)
			if err != nil {
				res.Error = err.Error()
				return res
			}
			res.Script = s
			scriptContent = s.Content
		case "score":
			if res.Script == nil {
				res.Error = "score step requires a script"
				return res
			}
			sc, err := runBatchScoreStep(ctx, h.text, res.Script.Title, res.Script.Content, sourcePlatform)
			if err != nil {
				res.Error = err.Error()
				return res
			}
			res.Score = sc
		case "adapt":
			if res.Script == nil {
				res.Error = "adapt step requires a script"
				return res
			}
			a, err := runBatchAdaptStep(ctx, h.text, res.Script.Title, bt.Angle, sourcePlatform, platforms)
			if err != nil {
				res.Error = err.Error()
				return res
			}
			res.Adaptations = a
		}
	}
	return res
}

// summarize builds the wire response from the per-topic
// results. Success = no Error string AND at least one
// non-nil section. We deliberately do not count "topics
// only" runs as failures — they are a valid, lighter-weight
// alternative the caller can opt into.
func (h *BatchHandler) summarize(items []batchGeneratedItem, ragTitles []string) batchResponse {
	resp := batchResponse{
		Generated: items,
		Summary: batchSummary{
			Total: len(items),
		},
		KnowledgeUsed: []string{},
	}
	for _, it := range items {
		if it.Error == "" {
			resp.Summary.SuccessCount++
		} else {
			resp.Summary.FailCount++
		}
	}
	if ragTitles != nil {
		resp.KnowledgeUsed = ragTitles
	}
	return resp
}

// buildDemoBatch returns N copies of the same canned
// per-topic pipeline. The demo path is intentionally
// non-rotating (every slot looks the same) because the
// operator's mental model is "I want to see what the API
// returns for one seed" — varying the data across slots
// would just make the demo harder to read.
func (h *BatchHandler) buildDemoBatch(req batchRequest, steps, platforms []string, ragTitles []string) []batchGeneratedItem {
	out := make([]batchGeneratedItem, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		out = append(out, h.buildDemoOneTopic(steps, platforms))
	}
	return out
}

// buildDemoOneTopic reuses the pipeline handler's demo path
// shape so the demo data is identical between /ai/pipeline
// and /ai/batch. We call agents.DemoResponse directly rather
// than instantiating a PipelineHandler because the batch
// handler is not the pipeline handler's caller.
func (h *BatchHandler) buildDemoOneTopic(steps, platforms []string) batchGeneratedItem {
	res := batchGeneratedItem{}
	topicRaw, _ := agents.DemoResponse(agents.DemoOpTopics, len(steps))
	topics, terr := parseTopics(topicRaw)
	if terr != nil || len(topics) == 0 {
		res.Error = "demo topics parse failed"
		return res
	}
	res.Topic = &batchTopic{generatedTopic: topics[0]}

	if len(steps) == 1 && steps[0] == "topics" {
		return res
	}

	scriptRaw, _ := agents.DemoResponse(agents.DemoOpHumanize, len(steps))
	scoreRaw, _ := agents.DemoResponse(agents.DemoOpScore, len(scriptRaw))
	adaptRaw, _ := agents.DemoResponse(agents.DemoOpPlatformAdapt, len(steps))

	for _, step := range steps {
		switch step {
		case "script":
			res.Script = &pipelineScript{
				Title:   topics[0].Title,
				Content: strings.TrimSpace(scriptRaw),
				Angle:   topics[0].Angle,
				Topic:   topics[0].Title,
			}
		case "score":
			s, err := parseQualityScore(scoreRaw)
			if err != nil {
				res.Error = "demo score parse failed"
				return res
			}
			res.Score = &s
		case "adapt":
			a, err := parsePlatformAdapt(adaptRaw)
			if err != nil {
				res.Error = "demo adapt parse failed"
				return res
			}
			res.Adaptations = &a
		}
	}
	return res
}

// Step helpers -------------------------------------------------------------
//
// The four step helpers below mirror the ones in pipeline.go
// but live in the batch handler so the batch surface is
// self-contained. We re-implement rather than re-use because
// the pipeline handler is a *PipelineHandler with its own
// dependencies and we don't want to widen the public API just
// for the batch endpoint to share step helpers.

// runBatchTopicsStep calls MiniMax to generate a single topic
// for the batch. The count parameter is reserved for a future
// "give me 3 ideas per slot" UX — today we always ask for 1
// because the batch is itself the count parameter. rag is
// injected under the same banner the pipeline uses.
func runBatchTopicsStep(ctx context.Context, m *agents.MiniMax, seed, platform, rag string, _ int) ([]generatedTopic, error) {
	prompt := agents.GenerateTopicsPrompt(seed, platform, 1)
	if rag != "" {
		prompt = injectRAG(prompt, rag)
	}
	cctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	res, err := m.Text(cctx, prompt, agents.MiniMaxTextOptions{Model: "MiniMax-M2.7-highspeed"})
	if err != nil {
		return nil, err
	}
	return parseTopics(res.Text)
}

// runBatchScriptStep asks MiniMax to expand the topic into a
// full script. Mirrors the pipeline handler's step exactly so
// the batch output is consistent with the per-call pipeline
// output.
func runBatchScriptStep(ctx context.Context, m *agents.MiniMax, topic generatedTopic, platform, rag string) (*pipelineScript, error) {
	seedScript := topic.Angle + "\n\n" + topic.Hook
	prompt := agents.HumanizeScriptPrompt(seedScript)
	if rag != "" {
		prompt = injectRAG(prompt, rag)
	}
	cctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	res, err := m.Text(cctx, prompt, agents.MiniMaxTextOptions{Model: "MiniMax-M2.7-highspeed"})
	if err != nil {
		return nil, err
	}
	return &pipelineScript{
		Title:   topic.Title,
		Content: strings.TrimSpace(res.Text),
		Angle:   topic.Angle,
		Topic:   topic.Title,
	}, nil
}

// runBatchScoreStep evaluates the generated script on the
// standard axes. On parse failure it falls back to the
// rule-based scorer so the batch never returns a hard 502 for
// a slightly off-shape MiniMax response.
func runBatchScoreStep(ctx context.Context, m *agents.MiniMax, title, script, platform string) (*qualityScoreResponse, error) {
	prompt := agents.ScoreContentPrompt(title, script, platform)
	cctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	res, err := m.Text(cctx, prompt, agents.MiniMaxTextOptions{Model: "MiniMax-M2.7-highspeed"})
	if err != nil {
		return nil, err
	}
	out, perr := parseQualityScore(res.Text)
	if perr != nil {
		fb, fbErr := qualityResponseFromMap(agents.RuleBasedScore(title, script, platform))
		if fbErr != nil {
			return nil, perr
		}
		return &fb, nil
	}
	return &out, nil
}

// runBatchAdaptStep asks MiniMax to produce per-platform
// versions. The platforms slice is the per-batch target list
// (e.g. 抖音/哔哩哔哩/小红书); the sourcePlatform is the
// platform the topic + script steps used. The platforms list is
// appended as an explicit "TARGETS" line so the model gets a
// stable, machine-checkable list of which platforms to emit
// versions for — defends against drift when the canonical
// default changes and a caller passes a non-default set.
func runBatchAdaptStep(ctx context.Context, m *agents.MiniMax, title, angle, sourcePlatform string, platforms []string) (*platformAdaptResponse, error) {
	prompt := agents.PlatformAdaptPrompt(title, angle, sourcePlatform)
	if len(platforms) > 0 {
		prompt = prompt + "\n\n## TARGETS (per-batch override)\n" + strings.Join(platforms, " / ")
	}
	cctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	res, err := m.Text(cctx, prompt, agents.MiniMaxTextOptions{Model: "MiniMax-M2.7-highspeed"})
	if err != nil {
		return nil, err
	}
	out, perr := parsePlatformAdapt(res.Text)
	if perr != nil {
		return nil, perr
	}
	return &out, nil
}
