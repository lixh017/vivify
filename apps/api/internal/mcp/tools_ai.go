package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/models"
)

// resolveTextProvider returns the TextProvider the AI-backed tools
// should invoke. The lookup chain is:
//
//  1. The resolver's per-user pick (when a non-zero user context
//     is available — future: thread agent_id → owner_user_id).
//     Today the MCP server has no per-request user_id (tools run
//     under the configured agent identity), so this branch fires
//     only when a future caller threads a user_id through
//     ToolInput.
//  2. The server's configured default text provider.
//
// The function never returns nil: at minimum it returns
// agents.Echo so a misconfigured resolver cannot panic a tool.
func (s *Server) resolveTextProvider(ctx context.Context) agents.TextProvider {
	if s.resolver != nil {
		// userID=0 today. See comment above for the planned
		// agent_id → owner_user_id threading.
		if p, err := s.resolver.Text(ctx, 0, "all"); err == nil && p != nil {
			return p
		}
	}
	if s.text != nil {
		return s.text
	}
	return agents.Echo
}

// toolGenerateTopics implements opc_generate_topics. Asks the
// resolved text provider for N differentiated topic ideas for the
// "熊猫" IP and returns the raw model output as text.
func (s *Server) toolGenerateTopics(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.text == nil {
		return nil, ToolOutput{}, fmt.Errorf("text client not configured")
	}
	var payload struct {
		Seed     string `json:"seed"`
		Platform string `json:"platform"`
		Count    int    `json:"count"`
	}
	if err := decodeParams(in.Params, &payload); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid generate_topics payload: %w", err)
	}
	if payload.Count <= 0 {
		payload.Count = 5
	}
	prompt := agents.GenerateTopicsPrompt(payload.Seed, payload.Platform, payload.Count)
	provider := s.resolveTextProvider(ctx)
	text, _, err := provider.CompleteWithUsage(ctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, ToolOutput{}, fmt.Errorf("generate topics: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Text: text}, nil
}

// toolHumanizeScript implements opc_humanize_script. Loads the
// target script, asks the resolved text provider to rewrite it so
// it does not feel AI-generated, and persists the rewritten content
// back to the DB.
func (s *Server) toolHumanizeScript(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.text == nil || s.db == nil {
		return nil, ToolOutput{}, fmt.Errorf("server not fully configured")
	}
	id, err := readID(in.Params, "script_id")
	if err != nil {
		return nil, ToolOutput{}, err
	}
	var sc models.Script
	if err := s.db.WithContext(ctx).First(&sc, id).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("load script: %w", err)
	}
	prompt := agents.HumanizeScriptPrompt(sc.Content)
	provider := s.resolveTextProvider(ctx)
	text, _, err := provider.CompleteWithUsage(ctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, ToolOutput{}, fmt.Errorf("humanize script: %w", err)
	}
	sc.Content = text
	if err := s.db.WithContext(ctx).Save(&sc).Error; err != nil {
		return nil, ToolOutput{}, fmt.Errorf("save humanized script: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Script: &sc}, nil
}

// toolDeconstructViral implements opc_deconstruct_viral. Asks the
// resolved text provider to break down a viral piece of content
// (by URL or raw text) and returns a structured AnalysisResult
// that the 复盘 dashboard can render.
func (s *Server) toolDeconstructViral(ctx context.Context, in ToolInput) (*mcpsdk.CallToolResult, ToolOutput, error) {
	if s.text == nil {
		return nil, ToolOutput{}, fmt.Errorf("text client not configured")
	}
	var payload struct {
		URL string `json:"url"`
		Raw string `json:"raw"`
	}
	if err := decodeParams(in.Params, &payload); err != nil {
		return nil, ToolOutput{}, fmt.Errorf("invalid deconstruct_viral payload: %w", err)
	}
	if payload.URL == "" && payload.Raw == "" {
		return nil, ToolOutput{}, fmt.Errorf("either url or raw must be provided")
	}
	prompt := buildDeconstructPrompt(payload.URL, payload.Raw)
	provider := s.resolveTextProvider(ctx)
	text, _, err := provider.CompleteWithUsage(ctx, prompt, agents.CompleteOptions{})
	if err != nil {
		return nil, ToolOutput{}, fmt.Errorf("deconstruct viral: %w", err)
	}
	return &mcpsdk.CallToolResult{}, ToolOutput{Analysis: &AnalysisResult{
		Hook:        extractSection(text, "钩子"),
		Structure:   extractSection(text, "结构"),
		WhyViral:    extractSection(text, "为什么爆"),
		Adaptation:  extractSection(text, "借鉴"),
		RawResponse: text,
	}}, nil
}

// buildDeconstructPrompt is the prompt template used by the viral
// deconstruction tool. Extracted so the test suite can verify the
// shape without spinning up the SDK.
func buildDeconstructPrompt(url, raw string) string {
	body := raw
	if body == "" {
		body = "(URL: " + url + ")"
	}
	return fmt.Sprintf(`请拆解以下爆款内容的创作手法：

%s

请从四个角度分析：
- 钩子（前 3 秒如何抓人）
- 结构（叙事节奏、铺垫 / 反转 / 收尾）
- 为什么爆（情绪 / 争议 / 共鸣 / 信息差）
- 借鉴（我们"熊猫"IP 怎么把这个套路本土化）

请按"钩子：\n结构：\n为什么爆：\n借鉴："四个标签分块输出。`, body)
}
