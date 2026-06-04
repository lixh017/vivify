package agents

import (
	"context"
	"fmt"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Default model + max-tokens for the OPC MCP server. Other tools
// (e.g. viral deconstruction) can override these per-call through
// CompleteOptions.
const (
	DefaultModel     = anthropic.ModelClaudeSonnet4_5
	DefaultMaxTokens = 4096
	DefaultTimeout   = 60 * time.Second
)

// Claude wraps the Anthropic SDK and exposes prompt builders and a
// Complete method used by the OPC MCP server for AI-assisted tasks
// (topic generation, script humanization, viral deconstruction).
type Claude struct {
	client        anthropic.Client
	keyConfigured bool         // false when constructed with empty API key (dev mode)
	override      CompleteFunc // optional test override; nil in production
	model         anthropic.Model
	maxTokens     int64
	timeout       time.Duration
}

// CompleteFunc lets tests inject a fake response without making a
// real network call. Production code never sets this.
type CompleteFunc func(ctx context.Context, prompt string) (string, error)

// CompleteOptions tunes a single Complete call. Zero values fall
// back to the Claude-constructor defaults. Use this when a specific
// tool needs a larger token budget or a different model.
type CompleteOptions struct {
	Model     anthropic.Model
	MaxTokens int64
	Timeout   time.Duration
}

// NewClaude constructs a Claude agent from an Anthropic API key.
// An empty key is rejected at construction so misconfiguration is
// caught at boot, not at first request. Use NewClaudeWithOverride
// for tests.
func NewClaude(apiKey string) *Claude {
	return NewClaudeWithOptions(apiKey, CompleteOptions{})
}

// NewClaudeWithOptions is like NewClaude but lets the caller
// override the model / max-tokens / timeout. Pass a CompleteOptions
// with all zero fields to get the same behaviour as NewClaude.
//
// An empty API key is allowed (dev mode): the returned Claude will
// have a nil client and Complete() will surface a clear "no API key"
// error at call time. This matches main.go's "empty key is OK during
// dev" contract and avoids taking the whole server down for a config
// issue. Tests that need a real call should use NewClaudeWithOverride.
func NewClaudeWithOptions(apiKey string, opts CompleteOptions) *Claude {
	c := &Claude{
		model:     DefaultModel,
		maxTokens: DefaultMaxTokens,
		timeout:   DefaultTimeout,
	}
	if apiKey != "" {
		c.client = anthropic.NewClient(option.WithAPIKey(apiKey))
		c.keyConfigured = true
	}
	if opts.Model != "" {
		c.model = opts.Model
	}
	if opts.MaxTokens > 0 {
		c.maxTokens = opts.MaxTokens
	}
	if opts.Timeout > 0 {
		c.timeout = opts.Timeout
	}
	return c
}

// NewClaudeWithOverride constructs a Claude agent that bypasses the
// Anthropic SDK entirely and uses the provided function as the
// completion source. Intended for tests; production code should
// always use NewClaude. The override bypasses the API-key check.
func NewClaudeWithOverride(fn CompleteFunc) *Claude {
	return &Claude{
		override:  fn,
		model:     DefaultModel,
		maxTokens: DefaultMaxTokens,
		timeout:   DefaultTimeout,
	}
}

// GenerateTopicsPrompt builds the prompt used to ask Claude for N
// differentiated topic ideas for the "熊猫" IP given a seed and target
// platform. The returned prompt is plain text (no tool-use wrapping).
func (c *Claude) GenerateTopicsPrompt(seed, platform string, count int) string {
	return fmt.Sprintf(
		`你是 OPC 的"熊猫"IP 选题助手。
赛道：治愈 / 御宅 / 哲学 / 国潮。
平台：%s。
基于以下种子概念：%s

请生成 %d 个差异化选题，每个包含：
- 标题（10-20 字）
- 角度（独特视角）
- 预期表现推理（为什么这条有机会爆）
- 钩子方向（前 3 秒如何抓人）

输出 JSON 数组。`,
		platform, seed, count,
	)
}

// HumanizeScriptPrompt builds the prompt used to ask Claude to rewrite
// a script so it does not feel AI-generated. It emphasizes removing
// AI traces (排比 / 三段式 / 过度铺垫) and adding human quirks
// (停顿 / 犹豫 / 转折 / 留白).
func (c *Claude) HumanizeScriptPrompt(script string) string {
	return fmt.Sprintf(
		`请把以下脚本改写得"不像 AI 写的"：
- 去掉工整对仗、完美结构
- 加入停顿、犹豫、转折
- 留白和呼吸感
- 去掉"AI 痕迹"：排比、三段式、过度铺垫
- 优先使用拟人化的语气、口语化表达

原脚本：
%s

输出改写后的版本。`,
		script,
	)
}

// Complete sends a single user-turn prompt to Claude and returns
// the first text content block. A per-call timeout is applied (see
// CompleteOptions) so a hung Anthropic call cannot pin a request
// goroutine indefinitely; callers can still impose a tighter
// deadline through ctx. Network/auth errors are returned to the
// caller as-is for surfacing.
func (c *Claude) Complete(ctx context.Context, prompt string) (string, error) {
	if c.override != nil {
		return c.override(ctx, prompt)
	}
	if !c.keyConfigured {
		return "", fmt.Errorf("claude complete: no Anthropic API key configured (set ANTHROPIC_API_KEY)")
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock(prompt),
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude complete: %w", err)
	}
	if len(resp.Content) == 0 {
		return "", nil
	}
	return resp.Content[0].Text, nil
}
