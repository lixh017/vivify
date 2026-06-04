package agents

import (
	"context"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Claude wraps the Anthropic SDK and exposes prompt builders and a
// Complete method used by the OPC MCP server for AI-assisted tasks
// (topic generation, script humanization, viral deconstruction).
type Claude struct {
	client anthropic.Client
}

// NewClaude constructs a Claude agent from an Anthropic API key.
// An empty key is allowed at construction time; failures happen at
// Complete-time.
func NewClaude(apiKey string) *Claude {
	return &Claude{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
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

// Complete sends a single user-turn prompt to Claude (claude-sonnet-4-5)
// and returns the first text content block. Network/auth errors are
// returned to the caller as-is for surfacing.
func (c *Claude) Complete(ctx context.Context, prompt string) (string, error) {
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_5,
		MaxTokens: 4096,
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
