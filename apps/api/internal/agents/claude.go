package agents

import (
	"context"
	"errors"
	"fmt"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ErrNoAPIKey is returned by Complete when the Claude agent was
// constructed without an Anthropic API key (dev mode / no
// configuration). Handlers should use errors.Is to detect it
// rather than sniffing err.Error() — substring matching is fragile
// and would silently mis-route any future error message that
// happens to contain the phrase "API key" (e.g. a Claude-side
// "invalid API key for org X" or a proxy "API key rejected").
//
// The error is exported so handlers across packages can branch on
// it without depending on the agents package's internal wording.
var ErrNoAPIKey = errors.New("ANTHROPIC_API_KEY not configured")

// ErrNoContent is returned by Complete when the SDK response has
// zero content blocks. The previous behaviour returned ("", nil)
// which silently produced empty envelopes (e.g. {"text":""}) for
// callers that wrap the result in JSON. Surfacing it as a typed
// sentinel lets the handler log it and return a proper 502 to the
// frontend instead of shipping a successful but empty body.
var ErrNoContent = errors.New("claude returned empty content")

// Default model + max-tokens for the OPC MCP server. Other tools
// (e.g. viral deconstruction) can override these per-call through
// CompleteOptions.
const (
	DefaultModel     = ModelClaudeSonnet4_5
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
	model         string
	maxTokens     int64
	timeout       time.Duration
}

// CompleteFunc lets tests inject a fake response without making a
// real network call. Production code never sets this.
type CompleteFunc func(ctx context.Context, prompt string) (string, error)

// CompleteOptions tunes a single Complete call. Zero values fall
// back to the Claude-constructor defaults. Use this when a specific
// tool needs a larger token budget or a different model.
//
// Model is a plain string (not anthropic.Model) so the Provider
// interface stays vendor-neutral — the Claude-specific providers
// cast to anthropic.Model at the SDK call site.
type CompleteOptions struct {
	Model     string
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
//
// keyConfigured is set to true so tests that exercise the real
// "Complete" path are NOT silently rerouted into demo mode. Tests
// that specifically want to exercise the demo path should construct
// the agent via NewClaude("") instead.
func NewClaudeWithOverride(fn CompleteFunc) *Claude {
	return &Claude{
		override:      fn,
		keyConfigured: true,
		model:         DefaultModel,
		maxTokens:     DefaultMaxTokens,
		timeout:       DefaultTimeout,
	}
}

// GenerateTopicsPrompt builds the prompt used to ask Claude for N
// differentiated topic ideas for the "熊猫" IP given a seed and
// target platform. The prompt is now IP-voice-anchored, gives a
// 7-pattern hook library, asks for a thinking pass before output,
// and ships with a worked example so the model does not regress to
// generic Chinese short-video topics.
//
// The returned prompt is plain text (no tool-use wrapping). The
// shape requested is a JSON array of objects matching TopicIdea.
func (c *Claude) GenerateTopicsPrompt(seed, platform string, count int) string {
	return fmt.Sprintf(
		`%s

## 任务
基于种子概念 "%s",为目标平台 "%s" 一次性生成 %d 个**差异化**的选题。要求:
- 4 个调性维度(治愈/御宅/哲学/国潮)中至少覆盖 3 个
- 每个选题必须能用一句话说清楚"和已有内容比,差别在哪"
- 标题字数严格遵守目标平台上限(抖音 ≤22 / B 站 ≤80 / 小红书 ≤20)

%s

%s

## 输出格式 (JSON array,严格遵守):
[
  {
    "title":               "10-20 字标题,符合平台字数",
    "angle":               "独特角度,1 句话,说明和同主题已有内容的差别",
    "hook":                "前 3 秒的具体台词+画面(画面用'画面:'前缀,旁白用'旁白:'前缀)",
    "expected_performance":"为什么这条有机会爆,给出情绪/数据/时令的具体推理",
    "pattern":             "用了哪个钩子公式(从上面的 7 个公式里挑一个写名字)",
    "voice_tags":          ["治愈","哲学"]  ← 命中了哪几个调性维度
  }
]

## 唯一示例(供参考,不要照抄):
种子 = "夜", 平台 = "抖音":
[
  {
    "title": "凌晨 3 点,熊猫为什么不睡",
    "angle": "御宅+治愈:深夜的孤独感用'反问自己'替代'心疼观众'",
    "hook": "画面: 窗边熊猫对窗发呆,3 秒没有台词。 旁白: 你也还没睡吧。",
    "expected_performance": "深夜流量高峰(00:00-03:00 抖音活跃度高)+ 孤独感共鸣 + 完播率因留白而拉满",
    "pattern": "画面钩子",
    "voice_tags": ["御宅","治愈"]
  }
]

%s`,
		PandaIPVoice,
		seed, platform, count,
		HookFormulaLibrary,
		CoTStepsTopics,
		OutputJSONOnly,
	)
}

// HumanizeScriptPrompt builds the prompt used to ask Claude to
// rewrite a script so it does not feel AI-generated. The new
// version anchors the IP voice up front, enumerates the anti-pattern
// list explicitly, and gives a before/after example so Claude sees
// exactly what "human" looks like in the panda IP.
func (c *Claude) HumanizeScriptPrompt(script string) string {
	return fmt.Sprintf(
		`%s

## 任务
把下面这段脚本改写得"不像 AI 写的"。先想清楚它当前的最大 AI 痕迹在哪里(排比?铺垫?口号?抽象?),再针对性改写,不要通篇平均用力。

## 改写要求(保留内容,只改语气与节奏):
- 留下具体画面(窗、雨、书、咖啡)而不是抽象概念
- 加入停顿/犹豫/转折:用 "..."、"(停顿)"、"(叹气)" 等显式标记
- 删掉"首先/其次/最后""大家好""让我们一起""加油"等模板句
- 短句优先,长句拆成两行;不写排比、不写三段式
- 结尾允许悬而未决,不强行收束成"愿你被世界温柔以待"
- 主体旁白以熊猫为视角(我/熊猫/它),不要第三人称讲解
- 如果原脚本超过 250 字,保留 60-80%% 的核心信息,不要全塞回去

%s

## 唯一示例(必须学这个感觉):
改写前(典型 AI 风):
"今天想给大家讲讲庄子。在如今这个内卷的时代,我们每个人都很焦虑。庄子告诉我们,真正逍遥的人,是放下执念的人。让我们一起做这样的逍遥人。"

改写后(熊猫 IP 风):
"你有没有想过——'逍遥'到底是什么?
(熊猫翻开《庄子》,慢慢念)
'乘天地之正,而御六气之辩,以游无穷者。'
嗯...其实我也没太懂。
但是吧,大概意思是:别太较劲。
风往哪吹,就往哪走一会儿。"

## 待改写脚本:
%s

## 输出
直接输出改写后的纯文本脚本,不要加任何解释/标题/前后缀。`,
		PandaIPVoice,
		PandaAntiPatterns,
		script,
	)
}

// PostmortemPrompt builds the prompt used to ask Claude to
// deconstruct a viral content item: why it performed well, what
// patterns are reusable, what the metrics suggest, and what to try
// next. The new version asks Claude to score the four dimensions
// (hook / structure / platform_fit / emotional_arc) and to
// separate "evidence" from "speculation" in the insights.
func (c *Claude) PostmortemPrompt(title, script, metrics, topicAngle string) string {
	return fmt.Sprintf(
		`%s

## 任务
对一条已经发布的"熊猫"内容做结构化复盘:先把每条结论的"证据"列清楚,再下判断,不要凭空表扬。

## 输入
- 标题: %s
- 脚本: %s
- 表现数据: %s
- 选题角度: %s

## 思考步骤
1. 先从数据里挑 2 个最异常的指标(显著高于或低于赛道均值),解释原因
2. 回到脚本,定位具体哪一段/哪一句台词和这两个指标对应
3. 提炼 3-5 条"换主题也能复用"的模式(必须是模式,不是表面技巧)
4. 给出 3 条下次可执行的改进,每条都要说"改哪里→为什么→预期效果"

%s

## 输出格式(JSON object,严格遵守以下 shape):
{
  "success_factors":   ["string", ...]   ← 3-5 条,每条都是一句"具体证据 + 结论",例:"前 3 秒'窗边的熊猫看着雨'有具象画面,完播率 68%% 验证钩子有效"
  "reusable_patterns": ["string", ...]   ← 3-5 条,必须是"换主题也能套"的模式,例:"治愈场景 = 自然元素(雨/雪/晚风) + 熊猫静态画面 + 一句话旁白"
  "insights":          ["string", ...]   ← 3-5 条,每条都是"claim | evidence | confidence"三段式,例:"完播率高于均值 23pct | 播放完成率 68%% vs 赛道 45%% | confidence=high"
  "axis_scores": {
    "hook":          0-100,
    "structure":     0-100,
    "platform_fit":  0-100,
    "emotional_arc": 0-100
  },
  "suggestions":       ["string", ...]   ← 3-5 条,每条都是"改哪里 → 为什么 → 预期效果"三段式,例:"结尾画面再多停 1 秒 → 给观众反应时间,留白提升评论区互动 → 预期评论量 +15pct"
}

%s`,
		PandaIPVoice,
		title, script, metrics, topicAngle,
		CoTStepsQuality,
		OutputJSONOnly,
	)
}

// Name implements agents.Provider for the legacy *Claude shim.
// New code should construct ClaudeProvider directly and let the
// Router pick the model; *Claude is kept for the existing handlers
// (handlers/ai.go, handlers/quality.go, handlers/deconstruct.go,
// handlers/pipeline.go, mcp/server.go) that own the prompt
// builders and only need a single-provider single-model surface.
func (c *Claude) Name() string { return ProviderClaude }

// Available implements agents.Provider for the legacy *Claude shim.
// Mirrors keyConfigured so dev mode is observable from the Provider
// surface (router fallback path).
func (c *Claude) Available() bool { return c.keyConfigured }

// CompleteWithOptions is the Provider-style variant of Complete:
// it takes a CompleteOptions struct so the router can pin per-task
// model + max-tokens + timeout without mutating the agent. The
// existing Complete(ctx, prompt) remains the single source of
// truth for the legacy handlers; this method just lets the router
// satisfy the Provider interface against the legacy *Claude.
func (c *Claude) CompleteWithOptions(ctx context.Context, prompt string, opts CompleteOptions) (string, error) {
	if c.override != nil {
		return c.override(ctx, prompt)
	}
	if !c.keyConfigured {
		return "", fmt.Errorf("claude complete: %w", ErrNoAPIKey)
	}
	model := c.model
	if opts.Model != "" {
		model = opts.Model
	}
	maxTokens := c.maxTokens
	if opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}
	timeout := c.timeout
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := c.client.Messages.New(cctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
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
		return "", fmt.Errorf("claude complete: %w", ErrNoContent)
	}
	return resp.Content[0].Text, nil
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
		// Wrap ErrNoAPIKey so handlers can branch on it via
		// errors.Is without depending on the exact phrasing.
		return "", fmt.Errorf("claude complete: %w", ErrNoAPIKey)
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
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
		// Surface as a typed sentinel so handlers can log a
		// meaningful diagnostic and return a real 502 — the
		// previous ("", nil) silently produced empty JSON
		// envelopes for callers that wrap the result.
		return "", fmt.Errorf("claude complete: %w", ErrNoContent)
	}
	return resp.Content[0].Text, nil
}
