package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// defaultListLimit is the maximum number of rows returned by the
// list-style tools (opc_list_topics). Callers can narrow it later
// with explicit filter parameters.
const defaultListLimit = 50

// decodeParams fills dst from a snake_case JSON parameter map. The
// map is re-encoded once into a single JSON object and then decoded
// into dst, which is cheaper than a per-key stream (a typical
// payload is 3-8 keys and the encoder reuses the same buffer pool).
func decodeParams(params map[string]json.RawMessage, dst any) error {
	if len(params) == 0 {
		return nil
	}
	buf, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf, dst)
}

// readID extracts a positive integer ID from the parameter map.
// Returns a descriptive error if the key is missing, non-numeric, or
// non-positive. Shared by every *_id-bearing tool.
func readID(params map[string]json.RawMessage, key string) (uint, error) {
	raw, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("missing required parameter %q", key)
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, fmt.Errorf("parameter %q must be a number: %w", key, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("parameter %q must be positive", key)
	}
	return uint(v), nil
}

// extractSection pulls a labelled block out of the model's free-text
// answer. Missing sections return an empty string so the JSON shape
// stays stable for downstream parsing.
func extractSection(text, label string) string {
	prefix := label + "："
	idx := strings.Index(text, prefix)
	if idx < 0 {
		return ""
	}
	rest := text[idx+len(prefix):]
	cutoff := len(rest)
	for _, next := range []string{"钩子：", "结构：", "为什么爆：", "借鉴："} {
		if next == prefix {
			continue
		}
		if i := strings.Index(rest, next); i >= 0 && i < cutoff {
			cutoff = i
		}
	}
	return strings.TrimSpace(rest[:cutoff])
}
