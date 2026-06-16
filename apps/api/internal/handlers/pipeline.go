package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/config"
)

// PipelineHandler exposes the multi-step "one-shot" content
// pipeline endpoint:
//
//	POST /ai/pipeline
//
// In a single call it runs:
//
//	topics  -> pick the best topic -> script -> score -> adapt
//
// ...subject to the `steps` filter the caller supplies. The RAG
// step is opt-in via `include_knowledge` and is implemented as a
// pre-step that augments the topic + script prompts with a
// "STYLE REFERENCE" block pulled from the user's knowledge base.
//
// Phase 4 wiring: the handler holds a textClientResolver
// (production) + a defaultText fallback (tests). The BatchHandler
// (Task 6) shares the same provider by calling Resolver() and
// building a per-call factory.
type PipelineHandler struct {
	resolver    textClientResolver
	defaultText agents.TextProvider
	db          *gorm.DB
	logger      *slog.Logger
}

// NewPipelineHandler wires a PipelineHandler that resolves the text
// provider per-request from the supplied resolver. db is required
// for the RAG step; if it is nil the handler still works (the RAG
// step is silently skipped) but the demo / live paths will fall
// through. Tests that don't need RAG can pass nil.
func NewPipelineHandler(resolver textClientResolver, db *gorm.DB) *PipelineHandler {
	return &PipelineHandler{
		resolver: resolver,
		db:       db,
		logger:   slog.Default(),
	}
}

// NewPipelineHandlerWithDefault is a transitional constructor
// kept during the Phase 4 migration. It wires the handler with a
// fixed TextProvider and no resolver, used by tests that have not
// been migrated to a stub resolver yet. New callers should prefer
// NewPipelineHandler(resolver, db).
func NewPipelineHandlerWithDefault(text agents.TextProvider, db *gorm.DB) *PipelineHandler {
	return &PipelineHandler{
		defaultText: text,
		db:          db,
		logger:      slog.Default(),
	}
}

// NewPipelineHandlerWithResolverAndDefault wires the full
// Phase 4 chain. See NewAIHandlerWithResolverAndDefault for
// the rationale; the pipeline's per-step helpers (runTopicsStep
// etc.) consume the resolved text directly, so leaving the
// resolver nil would silently route every live-path step to
// the configured default even when a per-user credential was
// available.
func NewPipelineHandlerWithResolverAndDefault(resolver textClientResolver, text agents.TextProvider, db *gorm.DB) *PipelineHandler {
	return &PipelineHandler{
		resolver:    resolver,
		defaultText: text,
		db:          db,
		logger:      slog.Default(),
	}
}

// RegisterRoutes attaches the pipeline endpoint to the router.
func (h *PipelineHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/ai/pipeline", h.RunPipeline)
}

// Text returns the configured default text provider. The pipeline
// doesn't have a per-request gin context inside the per-step
// helpers (they pass context.Context, not *gin.Context), so the
// step methods fall back to the configured default. Batch (Task 6)
// wires the resolver via Resolver() to build a per-tick factory so
// each batch run resolves with the current credentials.
func (h *PipelineHandler) Text() agents.TextProvider {
	return h.defaultText
}

// Resolver exposes the text resolver so the batch handler
// (Task 6) can build a per-call factory. Has no callers in
// Task 5; it lands as a forward-declared seam. Returns the
// interface type so callers don't depend on the agents package
// directly.
func (h *PipelineHandler) Resolver() textClientResolver {
	return h.resolver
}

// shouldUseDemo mirrors AIHandler.shouldUseDemo: demo mode kicks in
// when the caller passes ?demo=true or when the resolved provider
// reports it is not configured. Duplicated rather than shared so
// handlers stay decoupled.
func (h *PipelineHandler) shouldUseDemo(c *gin.Context) bool {
	if isDemoRequest(c) {
		return true
	}
	text := resolveTextForRequest(c, h.resolver, h.defaultText)
	return !text.Available()
}

// pipelineRequest is the JSON body for POST /ai/pipeline.
//
// seed is the starting concept the operator hands the machine;
// platform is the source platform for the topic + script steps
// (the adapt step always emits all three downstream platforms);
// include_knowledge gates the RAG lookup against knowledge_docs;
// steps is the explicit allow-list the caller can use to skip
// (e.g. the operator only wants the topic list, or already has a
// script and just wants the score + adapt). steps defaults to the
// full four-step pipeline when empty.
type pipelineRequest struct {
	Seed             string   `json:"seed"`
	Platform         string   `json:"platform"`
	IncludeKnowledge bool     `json:"include_knowledge"`
	Steps            []string `json:"steps"`
}

// pipelineResponse is the wire shape returned by /ai/pipeline.
// Each sub-object is omitempty so the caller only sees the steps
// they asked for. knowledge_used is always present (possibly
// empty) because the field is part of the demo's "transparency"
// promise — the frontend can show "grounded in N docs" even when
// the answer is zero.
type pipelineResponse struct {
	Topics        []generatedTopic       `json:"topics,omitempty"`
	Script        *pipelineScript        `json:"script,omitempty"`
	Score         *qualityScoreResponse  `json:"score,omitempty"`
	Adaptations   *platformAdaptResponse `json:"adaptations,omitempty"`
	KnowledgeUsed []string               `json:"knowledge_used"`
}

// pipelineScript is the trimmed script shape the pipeline emits.
// We deliberately do NOT return a full models.Script (with
// timestamps, user_id, etc.) — the pipeline is a generation
// surface, not a persistence surface. Callers who want to save
// the script can POST it to /scripts.
type pipelineScript struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Angle   string `json:"angle"`
	Topic   string `json:"topic"`
}

// allowedPipelineSteps is the set of step names the handler
// accepts. Anything outside this set is ignored (with a log line)
// so a typo in the caller's steps array does not silently drop
// the request — the unknown name is dropped and the rest still
// runs.
var allowedPipelineSteps = map[string]bool{
	"topics": true,
	"script": true,
	"score":  true,
	"adapt":  true,
}

// RunPipeline — POST /ai/pipeline
//
// Body: {seed, platform, include_knowledge, steps}
// 200:  {topics?, script?, score?, adaptations?, knowledge_used}
// 400:  missing seed / platform, empty steps after normalization
// 502:  AI demo response could not be parsed
// 503:  Claude is not configured and no demo flag
//
// Demo mode (?demo=true or no API key) short-circuits to a
// hardcoded full pipeline output so the operator can show the
// feature in a sales / investor context without burning tokens.
// The hardcoded output is the union of the first entry in each
// of DemoTopicsJSON / DemoHumanizedScripts / DemoQualityScores /
// DemoPlatformAdapts, glued together by the same step order the
// live path uses.
func (h *PipelineHandler) RunPipeline(c *gin.Context) {
	var req pipelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid pipeline body", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Seed = strings.TrimSpace(req.Seed)
	req.Platform = strings.TrimSpace(req.Platform)
	if req.Seed == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "seed is required"})
		return
	}
	if req.Platform == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "platform is required"})
		return
	}

	// Normalize the steps list. An empty / nil list means "all
	// four"; otherwise we keep the caller's order but drop names
	// that are not in allowedPipelineSteps. Order matters here
	// because the script step depends on topics, score on script,
	// and adapt on script — the test suite asserts the produced
	// order matches the input order so a caller that supplies
	// ["score", "topics"] gets score first then topics (and the
	// score step sees an empty best topic).
	steps := normalizePipelineSteps(req.Steps)
	if len(steps) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "steps must contain at least one of: topics, script, score, adapt"})
		return
	}

	// RAG pre-step. Pulls up to 3 knowledge_docs that mention
	// the seed and packages them as a "STYLE REFERENCE" block. We
	// run this BEFORE the topic + script steps so the block is
	// ready for injection without each step having to re-query.
	var ragContext string
	var ragTitles []string
	if req.IncludeKnowledge {
		ragContext, ragTitles = agents.FindRelevantKnowledge(c.Request.Context(), h.db, req.Seed, req.Platform)
	}

	// Demo mode: short-circuit to the canned full pipeline. The
	// RAG block is still computed (and its titles returned) so
	// the demo's "grounded in docs X, Y, Z" UX is consistent with
	// the live path. The demo topics/script/score/adapt blocks
	// themselves are not affected by the RAG content — they come
	// from the same canned pools the per-endpoint demos use.
	if h.shouldUseDemo(c) {
		markDemoResponse(c)
		resp := h.buildDemoPipeline(c, req, steps, ragTitles)
		c.JSON(http.StatusOK, resp)
		return
	}

	// Live path. We run the steps in the (caller-supplied) order
	// and stop emitting new sections when a dependency is empty
	// (e.g. no topics returned -> no script -> no score -> no
	// adapt). The first error short-circuits with a 503; partial
	// progress is not returned because the frontend's modal would
	// be hard to render against an incomplete step chain.
	//
	// Resolve the text provider once at the top of the live path
	// so every step uses the same per-request pick. The resolver
	// branch handles user-configured credentials; the default +
	// Echo fallbacks keep the call site free of nil-deref concerns
	// when the handler was constructed without a default.
	text := resolveTextForRequest(c, h.resolver, h.defaultText)
	resp := pipelineResponse{}
	if ragTitles != nil {
		resp.KnowledgeUsed = ragTitles
	} else {
		// Always emit [] not null so the frontend can render the
		// "grounded in N docs" line uniformly.
		resp.KnowledgeUsed = []string{}
	}
	bestTopic := generatedTopic{}

	for _, step := range steps {
		switch step {
		case "topics":
			topics, usage, err := h.runTopicsStep(c.Request.Context(), text, req.Seed, req.Platform, ragContext)
			if err != nil {
				h.surfaceStepError(c, "topics", err)
				return
			}
			StampClaudeCost(c, config.SkillMiniMaxM27, usage.Input, usage.Output)
			resp.Topics = topics
			if len(topics) > 0 {
				bestTopic = topics[0]
			}
		case "script":
			if bestTopic.Title == "" {
				// Caller asked for the script step but either
				// skipped topics or topics returned empty. Surface
				// a 400 with a useful message instead of silently
				// emitting an empty script.
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "script step requires a non-empty topic; include 'topics' in steps first",
				})
				return
			}
			script, usage, err := h.runScriptStep(c.Request.Context(), text, bestTopic, req.Platform, ragContext)
			if err != nil {
				h.surfaceStepError(c, "script", err)
				return
			}
			StampClaudeCost(c, config.SkillMiniMaxM27, usage.Input, usage.Output)
			resp.Script = script
		case "score":
			if resp.Script == nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "score step requires a script; include 'script' in steps first",
				})
				return
			}
			score, usage, err := h.runScoreStep(c.Request.Context(), text, resp.Script.Title, resp.Script.Content, req.Platform)
			if err != nil {
				h.surfaceStepError(c, "score", err)
				return
			}
			StampClaudeCost(c, config.SkillMiniMaxM27, usage.Input, usage.Output)
			resp.Score = score
		case "adapt":
			if resp.Script == nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "adapt step requires a script; include 'script' in steps first",
				})
				return
			}
			adapt, usage, err := h.runAdaptStep(c.Request.Context(), text, resp.Script.Title, bestTopic.Angle, req.Platform)
			if err != nil {
				h.surfaceStepError(c, "adapt", err)
				return
			}
			StampClaudeCost(c, config.SkillMiniMaxM27, usage.Input, usage.Output)
			resp.Adaptations = adapt
		}
	}

	c.JSON(http.StatusOK, resp)
}

// normalizePipelineSteps returns the steps list in the caller's
// order, with any unknown names dropped. An empty input becomes
// the canonical four-step pipeline.
func normalizePipelineSteps(in []string) []string {
	if len(in) == 0 {
		return []string{"topics", "script", "score", "adapt"}
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !allowedPipelineSteps[s] {
			continue
		}
		out = append(out, s)
	}
	return out
}

// textUsage is the small token-accounting return the per-step
// helpers surface so the step loop in RunPipeline can stamp
// cost onto the call_log row. Defined here (not in the agents
// package) because it is a plumbing type for the handler layer.
type textUsage struct {
	Input  int
	Output int
}

// runTopicsStep calls MiniMax for N topics with RAG augmentation.
// The RAG context is injected under a "STYLE REFERENCE" banner
// when present, BEFORE the rest of the prompt content, so MiniMax
// sees it as authoritative style guidance.
//
// The text parameter is the per-request resolved TextProvider
// (resolver or default — never nil). Passing it explicitly instead
// of reading h.Text() keeps the step methods free of an implicit
// nil-deref when the handler was constructed with
// NewPipelineHandler(resolver, db) (resolver branch, defaultText
// left nil). It also matches the per-request semantics of
// shouldUseDemo, which already consults the resolver via
// resolveTextForRequest.
func (h *PipelineHandler) runTopicsStep(ctx context.Context, text agents.TextProvider, seed, platform, rag string) ([]generatedTopic, textUsage, error) {
	prompt := agents.GenerateTopicsPrompt(seed, platform, 5)
	if rag != "" {
		prompt = injectRAG(prompt, rag)
	}
	cctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	body, usage, err := text.CompleteWithUsage(cctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, textUsage{}, err
	}
	topics, perr := parseTopicsLegacy(body)
	if perr != nil {
		return nil, textUsage{Input: usage.InputTokens, Output: usage.OutputTokens}, perr
	}
	return topics, textUsage{Input: usage.InputTokens, Output: usage.OutputTokens}, nil
}

// runScriptStep asks MiniMax to expand the chosen topic into a
// full script. The RAG context is injected identically to the
// topic step; the script prompt is the same humanize prompt the
// /ai/humanize endpoint uses, but with the topic's title and
// angle prepended so the model knows what to expand.
//
// See runTopicsStep for why text is passed explicitly.
func (h *PipelineHandler) runScriptStep(ctx context.Context, text agents.TextProvider, topic generatedTopic, platform, rag string) (*pipelineScript, textUsage, error) {
	seedScript := topic.Angle + "\n\n" + topic.Hook
	prompt := agents.HumanizeScriptPrompt(seedScript)
	if rag != "" {
		prompt = injectRAG(prompt, rag)
	}
	cctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	body, usage, err := text.CompleteWithUsage(cctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, textUsage{}, err
	}
	trimmed := strings.TrimSpace(body)
	return &pipelineScript{
		Title:   topic.Title,
		Content: trimmed,
		Angle:   topic.Angle,
		Topic:   topic.Title,
	}, textUsage{Input: usage.InputTokens, Output: usage.OutputTokens}, nil
}

// runScoreStep evaluates the generated script on the standard
// hook / structure / platform_fit axes. No RAG injection here —
// the score prompt's value comes from the script content, not
// the IP style reference. (RAG would mostly teach MiniMax to
// prefer the style guide over the actual data; we keep scoring
// evidence-driven.)
//
// See runTopicsStep for why text is passed explicitly.
func (h *PipelineHandler) runScoreStep(ctx context.Context, text agents.TextProvider, title, script, platform string) (*qualityScoreResponse, textUsage, error) {
	prompt := agents.ScoreContentPrompt(title, script, platform)
	cctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	body, usageRaw, err := text.CompleteWithUsage(cctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, textUsage{}, err
	}
	usage := textUsage{Input: usageRaw.InputTokens, Output: usageRaw.OutputTokens}
	out, perr := parseQualityScore(body)
	if perr != nil {
		// Gracefully degrade to the rule-based scorer on parse
		// failure, matching the /ai/score handler. The frontend
		// gets a usable answer instead of a 502. We log the
		// original parse error AND the fallback error (if any) so
		// an operator can still see what the rule-based scorer
		// produced when debugging a regression.
		h.logger.Warn("pipeline score: parse failed, falling back to rule-based scorer",
			"err", perr.Error(),
			"title", title,
		)
		fb, fbErr := qualityResponseFromMap(agents.RuleBasedScore(title, script, platform))
		if fbErr != nil {
			h.logger.Error("pipeline score: rule-based fallback also failed",
				"parse_err", perr.Error(),
				"fallback_err", fbErr.Error(),
			)
			return nil, usage, perr
		}
		return &fb, usage, nil
	}
	return &out, usage, nil
}

// runAdaptStep asks MiniMax to produce 抖音/哔哩哔哩/小红书
// versions of the script. No RAG injection — the per-platform
// voice table is already in the prompt, and adding user-doc
// style guidance tends to make the model hedge rather than
// commit to a specific platform's voice.
//
// See runTopicsStep for why text is passed explicitly.
func (h *PipelineHandler) runAdaptStep(ctx context.Context, text agents.TextProvider, title, angle, sourcePlatform string) (*platformAdaptResponse, textUsage, error) {
	prompt := agents.PlatformAdaptPrompt(title, angle, sourcePlatform)
	cctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()
	body, usage, err := text.CompleteWithUsage(cctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, textUsage{}, err
	}
	out, perr := parsePlatformAdapt(body)
	if perr != nil {
		return nil, textUsage{Input: usage.InputTokens, Output: usage.OutputTokens}, perr
	}
	return &out, textUsage{Input: usage.InputTokens, Output: usage.OutputTokens}, nil
}

// injectRAG prepends the "STYLE REFERENCE" block to the prompt.
// The banner is Chinese to match the rest of the prompt voice
// and the "STYLE REFERENCE" anchor is a verbatim copy of the
// banner the spec asked for so a future log-grep for it will
// find every RAG-augmented call in one place.
func injectRAG(prompt, ragContent string) string {
	const banner = "STYLE REFERENCE from user's knowledge base:\n\n"
	const closer = "\n\n--- end of style reference ---\n\n"
	return banner + ragContent + closer + prompt
}

// surfaceStepError maps a step-level MiniMax error to a useful
// HTTP status. The "no API key" sentinel is surfaced as 503 with
// the same wording the other handlers use; everything else is
// also 503 with the raw error message. We deliberately do NOT
// 502 here — the pipeline is a higher-level surface and the
// individual step's parse / complete failures should not be
// visible to the operator.
func (h *PipelineHandler) surfaceStepError(c *gin.Context, step string, err error) {
	h.logger.Error("pipeline step failed", "step", step, "err", err.Error(), "request_id", c.GetString("request_id"))
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "AI service failed at step " + step + ": " + err.Error(),
		"step":  step,
	})
}

// buildDemoPipeline returns the canned full pipeline response
// from the existing demo pools. The RAG titles are passed
// through so the demo's "grounded in docs X, Y, Z" line is
// populated when the operator has seeded a knowledge base.
//
// We re-parse the canned JSON so the response shape is
// guaranteed to match the live path (a regression in the demo
// pool would surface here, not in production). If a parse
// fails we fall back to a typed zero value so the operator
// still gets a usable 200.
func (h *PipelineHandler) buildDemoPipeline(c *gin.Context, req pipelineRequest, steps []string, ragTitles []string) pipelineResponse {
	resp := pipelineResponse{}
	if ragTitles != nil {
		resp.KnowledgeUsed = ragTitles
	} else {
		// Always emit [] not null so the frontend can render the
		// "grounded in N docs" line uniformly.
		resp.KnowledgeUsed = []string{}
	}

	topicRaw, _ := agents.DemoResponse(agents.DemoOpTopics, len(req.Seed))
	topics, terr := parseTopicsLegacy(topicRaw)
	if terr != nil {
		h.logger.Warn("pipeline demo: topics parse failed", "err", terr.Error(), "request_id", c.GetString("request_id"))
	}

	scriptRaw, _ := agents.DemoResponse(agents.DemoOpHumanize, len(req.Seed))
	scoreRaw, _ := agents.DemoResponse(agents.DemoOpScore, len(scriptRaw))
	adaptRaw, _ := agents.DemoResponse(agents.DemoOpPlatformAdapt, len(req.Seed))

	var bestTopic generatedTopic
	if len(topics) > 0 {
		bestTopic = topics[0]
	}
	score, serr := parseQualityScore(scoreRaw)
	if serr != nil {
		h.logger.Warn("pipeline demo: score parse failed", "err", serr.Error(), "request_id", c.GetString("request_id"))
	}
	adapt, aerr := parsePlatformAdapt(adaptRaw)
	if aerr != nil {
		h.logger.Warn("pipeline demo: adapt parse failed", "err", aerr.Error(), "request_id", c.GetString("request_id"))
	}

	for _, step := range steps {
		switch step {
		case "topics":
			resp.Topics = topics
		case "script":
			resp.Script = &pipelineScript{
				Title:   bestTopic.Title,
				Content: strings.TrimSpace(scriptRaw),
				Angle:   bestTopic.Angle,
				Topic:   bestTopic.Title,
			}
		case "score":
			s := score
			resp.Score = &s
		case "adapt":
			a := adapt
			resp.Adaptations = &a
		}
	}
	return resp
}
