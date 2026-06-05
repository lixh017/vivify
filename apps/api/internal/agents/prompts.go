package agents

// Centralized prompt catalog for the OPC "熊猫" IP.
//
// All prompt builders (claude.go, quality.go, deconstruct.go) draw
// from the constants defined here. To keep this file under the
// project's 800-line limit as the catalog grows, the shared blocks
// are split across sibling files:
//
//   - prompts_voice.go: PandaIPVoice, PandaAntiPatterns,
//     HookFormulaLibrary, PlatformVoice (the IP tone blocks).
//   - prompts_cot.go:   CoTStepsTopics, CoTStepsQuality,
//     CoTStepsDeconstruct (the chain-of-thought preambles).
//
// This file is intentionally small: it owns the per-task prompt
// builder docstrings and the JSON-only tail that every prompt
// appends. New tasks should add a new builder method in claude.go
// (or the appropriate task file) and pull the voice / CoT blocks
// from the sibling files.

// OutputJSONOnly is the boilerplate tail every prompt appends so
// Claude returns parseable JSON without markdown fences or
// preamble. Putting it in one place means a parser change (e.g.
// "now also allow json fences") only has to happen once.
const OutputJSONOnly = `只用 JSON 输出,不要加任何解释、不要 markdown 代码块、不要开场白。`
