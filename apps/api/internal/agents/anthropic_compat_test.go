package agents

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeAnthropic returns a server that echoes a canned Messages API
// response. The handler asserts the request shape is the Anthropic
// Messages API: model + max_tokens + messages.
func fakeAnthropic(t *testing.T, wantModel string, captured *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		*captured = string(body)
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			MaxTokens int `json:"max_tokens"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.Model != wantModel {
			http.Error(w, "wrong model", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_test",
			"type":  "message",
			"role":  "assistant",
			"model": req.Model,
			"content": []map[string]string{
				{"type": "text", "text": "hi from anthropic"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 11, "output_tokens": 22},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAnthropicCompatProvider_CompleteWithUsage(t *testing.T) {
	var captured string
	srv := fakeAnthropic(t, "claude-sonnet-4-5", &captured)

	p := NewAnthropicCompatProvider(AnthropicCompatConfig{
		APIKey:    "sk-test",
		BaseURL:   srv.URL,
		ModelName: "claude-sonnet-4-5",
	})

	text, usage, err := p.CompleteWithUsage(context.Background(), "hello", CompleteOptions{MaxTokens: 64})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if text != "hi from anthropic" {
		t.Errorf("want text=hi from anthropic, got %q", text)
	}
	if usage.InputTokens != 11 || usage.OutputTokens != 22 {
		t.Errorf("want usage=11/22, got %d/%d", usage.InputTokens, usage.OutputTokens)
	}
	if !strings.Contains(captured, `"model":"claude-sonnet-4-5"`) {
		t.Errorf("captured request missing model: %s", captured)
	}
}

func TestAnthropicCompatProvider_Available(t *testing.T) {
	// empty key → Available()==false
	p := NewAnthropicCompatProvider(AnthropicCompatConfig{ModelName: "x"})
	if p.Available() {
		t.Error("want Available()=false with no key")
	}
	p = NewAnthropicCompatProvider(AnthropicCompatConfig{APIKey: "sk-x", ModelName: "y"})
	if !p.Available() {
		t.Error("want Available()=true with key")
	}
}

func TestAnthropicCompatProvider_CompleteWithUsage_Handles4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": "rate limited"}`, http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	p := NewAnthropicCompatProvider(AnthropicCompatConfig{APIKey: "k", BaseURL: srv.URL, ModelName: "x"})
	_, _, err := p.CompleteWithUsage(context.Background(), "hi", CompleteOptions{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("want error mentioning 429, got %v", err)
	}
}

func TestAnthropicCompatProvider_CompleteWithUsage_HandlesMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)
	p := NewAnthropicCompatProvider(AnthropicCompatConfig{APIKey: "k", BaseURL: srv.URL, ModelName: "x"})
	_, _, err := p.CompleteWithUsage(context.Background(), "hi", CompleteOptions{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("want error mentioning parse, got %v", err)
	}
}

func TestAnthropicCompatProvider_CompleteWithUsage_RespectsBaseRespError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"base_resp":{"status_code":1008,"status_msg":"quota exceeded"},"content":[],"usage":{}}`))
	}))
	t.Cleanup(srv.Close)
	p := NewAnthropicCompatProvider(AnthropicCompatConfig{APIKey: "k", BaseURL: srv.URL, ModelName: "x"})
	_, _, err := p.CompleteWithUsage(context.Background(), "hi", CompleteOptions{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "1008") {
		t.Errorf("want error mentioning status_code 1008, got %v", err)
	}
}

func TestAnthropicCompatProvider_CompleteWithUsage_UnavailableReturnsSentinel(t *testing.T) {
	p := NewAnthropicCompatProvider(AnthropicCompatConfig{ModelName: "x"}) // no key
	_, _, err := p.CompleteWithUsage(context.Background(), "hi", CompleteOptions{})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("want errors.Is(err, ErrProviderUnavailable)=true, got %v", err)
	}
}
