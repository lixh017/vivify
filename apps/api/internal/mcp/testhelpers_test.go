package mcp

import "encoding/json"

// jsonMust marshals v and panics on error. Tests use it to build
// SDK call arguments inline; panics from this helper indicate a
// programming error, not a test failure, so failure is appropriate.
func jsonMust(v any) json.RawMessage {
	buf, err := jsonMarshal(v)
	if err != nil {
		panic(err)
	}
	return buf
}

// jsonMarshal is a thin alias around json.Marshal so the production
// server.go can stay free of test-only helpers. Tests import this
// file implicitly.
func jsonMarshal(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// jsonUnmarshal is the corresponding alias for json.Unmarshal. We
// only have a single test-only helper file so the test package
// surface stays small.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
