package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/models"
)

// Panda-tuned MCP tools.
//
// The 4 panda_* tools wrap the generic opc_* tools with the
// panda IP profile baked into the LLM prompt, so the caller
// (Claude Code) does not have to remember the 4-tone matrix,
// the outfit whitelist, or the voice-tone rules. They are
// the MCN-flavored defaults in tools_topics / tools_scripts /
// tools_ai.go with the IP voice pre-loaded.
//
// Tool surface:
//
//   panda_topic       — generate 5 candidate topics for a given
//                       voice tone + hook angle. Does NOT persist.
//                       Caller picks one and uses opc_create_topic.
//
//   panda_script      — write a script for a topic, persist it,
//                       return the new script id. Voice + duration
//                       + platform are baked in.
//
//   panda_voice       — read-only audit of a script (or raw text):
//                       voice mix, scenes, outfits, AI tells, IP
//                       compliance. Returns VoiceReport, does NOT
//                       rewrite — use opc_humanize_script for that.
//
//   panda_storyboard  — turn a script into a shot list with Kling
//                       prompts. Returns StoryboardReport; caller
//                       can paste shots into Kling / 即梦 directly.

// voiceParamRE is the validation regex for the 4 panda tones. We
// accept the exact 4 terms and a few common variants callers may
// type. Anything else fails fast at the handler level rather than
// reaching the LLM with a confusing prompt.
var voiceParamRE = regexp.MustCompile(`^(治愈|御宅|哲学|国潮|healing|otaku|philosophy|guochao)$`)

func normalizePandaVoice(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "healing":
		return "治愈"
	case "otaku":
		return "御宅"
	case "philosophy":
		return "哲学"
	case "guochao":
		return "国潮"
	}
	return strings.TrimSpace(v)
}

// toolPandaTopic implements panda_topic. Generates N topic ideas
// with the panda IP profile pre-loaded into the prompt, and
// returns the raw model JSON in the Text field. The caller is
// expected to pick one and follow up with opc_create_topic.
func (s *Server) toolPandaTopic(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.text == nil {
		return nil, ToolOutput{}, fmt.Errorf("text client not configured")
	}
	var payload struct {
		Voice     string `json:"voice"`
		HookAngle string `json:"hook_angle"`
		Platform  string `json:"platform"`
		Count     int    `json:"count"`
	}
	if err := decodeParams(in.Params, &payload); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid panda_topic payload: %w", err)
	}
	voice := normalizePandaVoice(payload.Voice)
	if !voiceParamRE.MatchString(voice) {
		return nil, ToolOutput{}, fmt.Errorf("voice must be one of 治愈/御宅/哲学/国潮 (got %q)", payload.Voice)
	}
	if strings.TrimSpace(payload.Platform) == "" {
		return nil, ToolOutput{}, fmt.Errorf("platform is required (e.g. 抖音 / 哔哩哔哩 / 小红书)")
	}
	if payload.Count <= 0 {
		payload.Count = 5
	}
	prompt := agents.PandaTopicPrompt(voice, payload.HookAngle, payload.Platform, payload.Count)
	provider := s.resolveTextProvider(ctx)
	text, _, err := provider.CompleteWithUsage(ctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, ToolOutput{}, fmt.Errorf("panda_topic: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Text: text}, nil
}

// toolPandaScript implements panda_script. Writes a script for
// the given topic with the IP profile + voice + duration budget
// baked in, persists it, and returns the new script row.
func (s *Server) toolPandaScript(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.text == nil || s.db == nil {
		return nil, ToolOutput{}, fmt.Errorf("server not fully configured")
	}
	var payload struct {
		TopicID     uint   `json:"topic_id"`
		Voice       string `json:"voice"`
		DurationSec int    `json:"duration_sec"`
		Platform    string `json:"platform"`
	}
	if err := decodeParams(in.Params, &payload); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid panda_script payload: %w", err)
	}
	if payload.TopicID == 0 {
		return nil, ToolOutput{}, fmt.Errorf("topic_id is required")
	}
	voice := normalizePandaVoice(payload.Voice)
	if !voiceParamRE.MatchString(voice) {
		return nil, ToolOutput{}, fmt.Errorf("voice must be one of 治愈/御宅/哲学/国潮 (got %q)", payload.Voice)
	}
	var topic models.Topic
	if err := s.db.WithContext(ctx).First(&topic, payload.TopicID).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("load topic: %w", err)
	}
	platform := payload.Platform
	if platform == "" {
		platform = topic.Platform
	}
	if !models.PlatformIsAllowed(platform) {
		return nil, ToolOutput{}, fmt.Errorf("platform %q is not allowed", platform)
	}
	if payload.DurationSec <= 0 {
		payload.DurationSec = 60
	}
	prompt := agents.PandaScriptPrompt(topic.Title, topic.Angle, voice, platform, payload.DurationSec)
	provider := s.resolveTextProvider(ctx)
	text, _, err := provider.CompleteWithUsage(ctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, ToolOutput{}, fmt.Errorf("panda_script: %w", err)
	}
	sc := models.Script{
		TopicID:  topic.ID,
		Title:    topic.Title,
		Content:  strings.TrimSpace(text),
		Platform: platform,
	}
	if err := s.db.WithContext(ctx).Create(&sc).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("persist panda_script: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Script: &sc}, nil
}

// toolPandaVoice implements panda_voice. Loads a script (or
// accepts raw text) and asks the LLM for a structured voice-tone
// audit. Read-only: does NOT modify the script.
func (s *Server) toolPandaVoice(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.text == nil {
		return nil, ToolOutput{}, fmt.Errorf("text client not configured")
	}
	var payload struct {
		ScriptID uint   `json:"script_id"`
		Text     string `json:"text"`
		Title    string `json:"title"`
		Platform string `json:"platform"`
	}
	if err := decodeParams(in.Params, &payload); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid panda_voice payload: %w", err)
	}
	if payload.ScriptID == 0 && strings.TrimSpace(payload.Text) == "" {
		return nil, ToolOutput{}, fmt.Errorf("either script_id or text is required")
	}
	var (
		title    = payload.Title
		platform = payload.Platform
		body     = payload.Text
	)
	if payload.ScriptID != 0 {
		if s.db == nil {
			return nil, ToolOutput{}, fmt.Errorf("script_id given but db not configured")
		}
		var sc models.Script
		if err := s.db.WithContext(ctx).First(&sc, payload.ScriptID).Error; err != nil {
			return nil, ToolOutput{}, fmt.Errorf("load script: %w", err)
		}
		if title == "" {
			title = sc.Title
		}
		if platform == "" {
			platform = sc.Platform
		}
		body = sc.Content
	}
	prompt := agents.PandaVoiceAnalysisPrompt(title, body, platform)
	provider := s.resolveTextProvider(ctx)
	text, _, err := provider.CompleteWithUsage(ctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, ToolOutput{}, fmt.Errorf("panda_voice: %w", err)
	}
	report, perr := parseVoiceReport(text)
	if perr != nil {
		// Parse failure is recoverable: surface the raw text so
		// the caller can still see what the model said. The
		// VoiceReport pointer stays nil; the raw text lives in
		// the Text field via ToolOutput marshaling.
		return &mcpsdk.CallToolResult{}, ToolOutput{Text: text}, nil
	}
	report.RawResponse = text
	return &mcpsdk.CallToolResult{}, ToolOutput{Voice: report, Text: text}, nil
}

// toolPandaStoryboard implements panda_storyboard. Loads the
// script, asks the LLM to split it into shots with Kling
// prompts, and returns the parsed shot list.
func (s *Server) toolPandaStoryboard(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.text == nil || s.db == nil {
		return nil, ToolOutput{}, fmt.Errorf("server not fully configured")
	}
	var payload struct {
		ScriptID uint   `json:"script_id"`
		Voice    string `json:"voice"`
	}
	if err := decodeParams(in.Params, &payload); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid panda_storyboard payload: %w", err)
	}
	if payload.ScriptID == 0 {
		return nil, ToolOutput{}, fmt.Errorf("script_id is required")
	}
	voice := normalizePandaVoice(payload.Voice)
	if !voiceParamRE.MatchString(voice) {
		return nil, ToolOutput{}, fmt.Errorf("voice must be one of 治愈/御宅/哲学/国潮 (got %q)", payload.Voice)
	}
	var sc models.Script
	if err := s.db.WithContext(ctx).First(&sc, payload.ScriptID).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("load script: %w", err)
	}
	prompt := agents.PandaStoryboardPrompt(sc.Content, voice, sc.Platform)
	provider := s.resolveTextProvider(ctx)
	text, _, err := provider.CompleteWithUsage(ctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, ToolOutput{}, fmt.Errorf("panda_storyboard: %w", err)
	}
	shots, perr := parseStoryboardShots(text)
	if perr != nil {
		// Same lenient-fallback policy as panda_voice: return
		// raw text on parse failure.
		return &mcpsdk.CallToolResult{}, ToolOutput{Text: text}, nil
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: text}},
	}, ToolOutput{
		Storyboard: &StoryboardReport{Voice: voice, Shots: shots},
		Text:       text,
	}, nil
}

// parseVoiceReport extracts a VoiceReport from the model's
// free-form answer. The model is asked to return strict JSON
// (OutputJSONOnly), but small models occasionally wrap the
// answer in prose; we try the whole string first, then the
// largest JSON object substring.
func parseVoiceReport(text string) (*VoiceReport, error) {
	candidates := extractJSONCandidates(text)
	var lastErr error
	for _, raw := range candidates {
		var r VoiceReport
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			lastErr = err
			continue
		}
		if r.DominantVoice != "" || len(r.Scenes) > 0 || len(r.AITells) > 0 {
			return &r, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no VoiceReport-shaped object found")
	}
	return nil, lastErr
}

// parseStoryboardShots extracts the shot array from the model's
// answer. Same fallback policy as parseVoiceReport.
func parseStoryboardShots(text string) ([]StoryboardShot, error) {
	candidates := extractJSONCandidates(text)
	var lastErr error
	for _, raw := range candidates {
		var arr []StoryboardShot
		if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
			return arr, nil
		}
		// also try a wrapped object: { "shots": [...] }
		var wrap struct {
			Shots []StoryboardShot `json:"shots"`
		}
		if err := json.Unmarshal([]byte(raw), &wrap); err == nil && len(wrap.Shots) > 0 {
			return wrap.Shots, nil
		}
		lastErr = fmt.Errorf("no shot array found")
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no StoryboardShot-shaped array found")
	}
	return nil, lastErr
}

// extractJSONCandidates returns the raw text plus every
// balanced {...} / [...] substring inside it. The model is
// supposed to return strict JSON, but we tolerate prose wrappers
// and code-fenced blocks.
func extractJSONCandidates(text string) []string {
	out := []string{text}
	for _, opener := range []string{"{", "["} {
		closer := "}"
		if opener == "[" {
			closer = "]"
		}
		idx := 0
		for {
			start := strings.Index(text[idx:], opener)
			if start < 0 {
				break
			}
			start += idx
			end := balancedEnd(text, start, opener == "[")
			if end <= start {
				idx = start + 1
				continue
			}
			out = append(out, text[start:end+1])
			idx = end + 1
		}
		_ = closer
	}
	return out
}

// balancedEnd returns the index of the matching closing bracket
// for the opener at start, scanning from start+1. Returns -1
// if the brackets do not balance (e.g. truncated response).
func balancedEnd(s string, start int, isArray bool) int {
	open, close := byte('{'), byte('}')
	if isArray {
		open, close = '[', ']'
	}
	depth := 0
	inStr := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' && inStr {
			escape = true
			continue
		}
		if c == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		if c == open {
			depth++
		} else if c == close {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
