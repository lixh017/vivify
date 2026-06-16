package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
)

// stdoutCaptureMu serializes the captureStdout swap of the
// global os.Stdout so parallel subtests don't race on the
// pointer (one subtest could restore the original between
// another's os.Stdout read and the caller's write, dropping
// or interleaving output).
//
// Pre-existing race: caught by `go test -race` on
// 2026-06-16. The fix preserves the per-subtest isolation
// (each still has its own pipe + buffer) while serializing
// only the global-pointer read+write window.
var stdoutCaptureMu sync.Mutex

// TestRunCheckAllTypes confirms the --type flag is wired and each
// of the 4 IP types produces a high ConsistencyResult for a
// hand-crafted prompt that hits all anchors.
func TestRunCheckAllTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		typeName string
		prompt   string
	}{
		{
			typeName: "anthropomorphic",
			prompt:   "峰哥, 成年熊猫, rgb(245,240,225), rgb(26,26,26), 国潮, 不露爪, 不要 AI 生成感",
		},
		{
			typeName: "digital_human",
			prompt:   "莉娜, 28 岁, female, 东亚, rgb(245,228,210), 温柔知性, 普通话, 表情自然, 无恐怖谷",
		},
		{
			typeName: "costume",
			prompt:   "纤云, 唐代, 侠女, 襦裙, 长剑, 无穿越, 朱红#C73E1D",
		},
		{
			typeName: "info",
			prompt:   "OPC 每日资讯, 演播室双主播, 主播阿橙, 黑体加粗, 暖色字幕, 宝蓝#1F5FA8, rgb(245,240,225), 无 AI 播报感",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.typeName, func(t *testing.T) {
			t.Parallel()

			args := []string{"--type", tc.typeName, "--prompt", tc.prompt}
			output := captureStdout(t, func() {
				if err := runCheck(args); err != nil {
					t.Fatalf("runCheck(%v) error: %v", args, err)
				}
			})
			if !strings.Contains(output, "0.99") && !strings.Contains(output, "1.00") && !strings.Contains(output, "\"score\": 1") {
				t.Errorf("output missing score >= 0.99: %s", output)
			}
		})
	}
}

// TestRunCheckDefaultsToAnthropomorphic confirms the no-flag
// case still works (back-compat with Phase 1 callers).
func TestRunCheckDefaultsToAnthropomorphic(t *testing.T) {
	t.Parallel()

	args := []string{"--prompt", "峰哥, 成年熊猫, rgb(245,240,225), rgb(26,26,26), 国潮, 不露爪, 不要 AI 生成感"}
	output := captureStdout(t, func() {
		if err := runCheck(args); err != nil {
			t.Fatalf("runCheck default error: %v", err)
		}
	})
	if !strings.Contains(output, "0.99") && !strings.Contains(output, "1.00") && !strings.Contains(output, "\"score\": 1") {
		t.Errorf("default --type should be anthropomorphic, output: %s", output)
	}
}

// TestRunTopicMissingFlags confirms runTopic returns an error
// when --seed/--platform/--out are missing. Does not depend
// on the LLM (no API key required).
func TestRunTopicMissingFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{"no flags", []string{"topic"}},
		{"only seed", []string{"topic", "--seed", "x"}},
		{"seed and platform, no out", []string{"topic", "--seed", "x", "--platform", "抖音"}},
		{"seed and out, no platform", []string{"topic", "--seed", "x", "--out", "/tmp/x.json"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := runTopic(tc.args, slog.Default())
			if err == nil {
				t.Fatalf("runTopic(%v) error = nil, want error", tc.args)
			}
		})
	}
}

// captureStdout redirects os.Stdout during fn, returns what was
// printed. Used for CLI tests that print to stdout instead of
// returning strings. The stdoutCaptureMu mutex serializes the
// global-pointer read+write window; the per-call pipe + buffer
// stay isolated so each caller still gets only its own output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	stdoutCaptureMu.Lock()
	defer stdoutCaptureMu.Unlock()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	return buf.String()
}
