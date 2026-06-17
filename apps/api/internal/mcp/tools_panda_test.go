package mcp

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/db"
)

// pandaServerWithEcho wires a Server whose text client echoes the
// input prompt back. The 4 panda tools all read the LLM output
// from the text client, so echoing the prompt is a useful way to
// verify the prompt was constructed correctly (echo contains the
// voice name, the IP profile block, the platform, etc.) without
// needing a real Claude API call.
func pandaServerWithEcho(t *testing.T) *Server {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	echo := agents.NewMiniMaxWithTextOverride(func(_ context.Context, prompt string, _ agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
		return &agents.MiniMaxTextResult{Text: prompt}, nil
	})
	s, err := NewServer(gormDB, echo, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

func TestPandaTopic_RequiresVoice(t *testing.T) {
	s := pandaServerWithEcho(t)
	_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name: "panda_topic",
			Arguments: jsonMust(map[string]any{
				"platform": "抖音", "count": 3,
			}),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "voice") {
		t.Errorf("expected voice error, got: %v", err)
	}
}

func TestPandaTopic_RejectsUnknownVoice(t *testing.T) {
	s := pandaServerWithEcho(t)
	_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name: "panda_topic",
			Arguments: jsonMust(map[string]any{
				"voice": "黑暗", "platform": "抖音",
			}),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "voice") {
		t.Errorf("expected voice error, got: %v", err)
	}
}

func TestPandaTopic_RequiresPlatform(t *testing.T) {
	s := pandaServerWithEcho(t)
	_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name: "panda_topic",
			Arguments: jsonMust(map[string]any{
				"voice": "治愈",
			}),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "platform") {
		t.Errorf("expected platform error, got: %v", err)
	}
}

func TestPandaTopic_PromptContainsIPProfile(t *testing.T) {
	// Verify the LLM prompt bakes in the IP profile + voice tone
	// guide + hook angle when given. We use the echo provider so
	// the Text field IS the prompt.
	s := pandaServerWithEcho(t)
	out, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name: "panda_topic",
			Arguments: jsonMust(map[string]any{
				"voice":      "治愈",
				"hook_angle": "凌晨 3 点窗边独处",
				"platform":   "抖音",
				"count":      5,
			}),
		},
	})
	if err != nil {
		t.Fatalf("panda_topic: %v", err)
	}
	tc, ok := out.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", out.Content[0])
	}
	body := tc.Text
	for _, want := range []string{
		"熊猫 OPC",      // IP profile anchor
		"治愈",          // voice echoed in prompt
		"凌晨 3 点窗边独处", // hook angle baked in
		"抖音",          // platform echoed
		"outfit_hufu_red", // 5-outfit whitelist reference
		"竹林小院",        // scene whitelist reference
	} {
		if !strings.Contains(body, want) {
			t.Errorf("prompt missing %q\n--- prompt ---\n%s", want, body)
		}
	}
}

func TestPandaScript_PersistsScript(t *testing.T) {
	// End-to-end: create a topic, call panda_script with a fake
	// Claude that returns canned script text, verify the script
	// row is persisted with the right fields.
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := db.Migrate(gormDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const canned = "嘿,来了啊。\n(停顿)\n外面的竹子在响。\n我也不太想说话。\n那就一起待着吧。"
	claude := agents.NewMiniMaxWithTextOverride(func(_ context.Context, _ string, _ agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
		return &agents.MiniMaxTextResult{Text: canned}, nil
	})
	s, err := NewServer(gormDB, claude, nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	topic := callTool(t, s, "opc_create_topic", map[string]any{
		"title": "凌晨 3 点独处", "angle": "治愈调", "platform": "抖音",
	})

	out, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name: "panda_script",
			Arguments: jsonMust(map[string]any{
				"topic_id":     topic.Topic.ID,
				"voice":        "治愈",
				"duration_sec": 30,
			}),
		},
	})
	if err != nil {
		t.Fatalf("panda_script: %v", err)
	}
	tc, ok := out.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", out.Content[0])
	}
	var body ToolOutput
	if err := jsonUnmarshal([]byte(tc.Text), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Script == nil {
		t.Fatalf("panda_script: nil script")
	}
	if body.Script.Content != canned {
		t.Errorf("panda_script content = %q, want %q", body.Script.Content, canned)
	}
	if body.Script.TopicID != topic.Topic.ID {
		t.Errorf("panda_script topic_id = %d, want %d", body.Script.TopicID, topic.Topic.ID)
	}
	if body.Script.Platform != "抖音" {
		t.Errorf("panda_script platform = %q, want 抖音 (defaulted from topic)", body.Script.Platform)
	}
}

func TestPandaVoice_AcceptsRawText(t *testing.T) {
	// panda_voice must accept raw text without a script_id so
	// the caller can audit a hand-written draft before saving.
	s := pandaServerWithEcho(t)
	const raw = "你有没有想过——'逍遥'到底是什么?\n(熊猫翻开《庄子》)\n'乘天地之正,而御六气之辩,以游无穷者。'"
	out, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name: "panda_voice",
			Arguments: jsonMust(map[string]any{
				"text":     raw,
				"title":    "逍遥",
				"platform": "哔哩哔哩",
			}),
		},
	})
	if err != nil {
		t.Fatalf("panda_voice: %v", err)
	}
	if len(out.Content) == 0 {
		t.Fatalf("panda_voice: empty content")
	}
}

func TestPandaVoice_RequiresEitherScriptIDOrText(t *testing.T) {
	s := pandaServerWithEcho(t)
	_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name:      "panda_voice",
			Arguments: jsonMust(map[string]any{}),
		},
	})
	if err == nil {
		t.Error("expected error for empty panda_voice payload")
	}
}

func TestPandaStoryboard_RequiresScriptID(t *testing.T) {
	s := pandaServerWithEcho(t)
	_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name: "panda_storyboard",
			Arguments: jsonMust(map[string]any{
				"voice": "治愈",
			}),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "script_id") {
		t.Errorf("expected script_id error, got: %v", err)
	}
}

func TestPandaStoryboard_RequiresVoice(t *testing.T) {
	s := pandaServerWithEcho(t)
	_, err := s.HandleTool(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name: "panda_storyboard",
			Arguments: jsonMust(map[string]any{
				"script_id": 1,
			}),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "voice") {
		t.Errorf("expected voice error, got: %v", err)
	}
}

func TestParseVoiceReport_AcceptsProseWrapped(t *testing.T) {
	// The model occasionally wraps the JSON in prose. Verify the
	// parser finds the object substring rather than failing.
	raw := `分析结果如下: {"voice_mix":{"治愈":0.6,"御宅":0.2,"哲学":0.1,"国潮":0.1},"dominant_voice":"治愈","scenes":["月下窗棂"],"ai_tells":[],"ip_compliance":{"passed":true,"violations":[],"notes":""},"recommendations":[""]}`
	r, err := parseVoiceReport(raw)
	if err != nil {
		t.Fatalf("parseVoiceReport: %v", err)
	}
	if r.DominantVoice != "治愈" {
		t.Errorf("dominant_voice = %q, want 治愈", r.DominantVoice)
	}
	if r.VoiceMix["治愈"] < 0.5 {
		t.Errorf("voice_mix[治愈] = %v, want ≥0.5", r.VoiceMix["治愈"])
	}
}

func TestParseStoryboardShots_AcceptsBareArray(t *testing.T) {
	raw := `[{"shot_id":1,"duration_sec":4,"scene":"竹林小院","outfit":"outfit_hufu_red","prop":"折扇","action":"峰哥背对镜头","voiceover":"嘿,来了啊","kling_prompt":"test"}]`
	shots, err := parseStoryboardShots(raw)
	if err != nil {
		t.Fatalf("parseStoryboardShots: %v", err)
	}
	if len(shots) != 1 || shots[0].Scene != "竹林小院" {
		t.Errorf("got %+v", shots)
	}
}

func TestParseStoryboardShots_AcceptsWrappedObject(t *testing.T) {
	raw := `{"shots":[{"shot_id":1,"duration_sec":4,"scene":"竹林小院","outfit":"outfit_hufu_red","prop":"折扇","action":"x","voiceover":"y","kling_prompt":"z"}]}`
	shots, err := parseStoryboardShots(raw)
	if err != nil {
		t.Fatalf("parseStoryboardShots (wrapped): %v", err)
	}
	if len(shots) != 1 {
		t.Errorf("got %d shots, want 1", len(shots))
	}
}
