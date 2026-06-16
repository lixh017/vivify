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

// fakeOpenAI returns a server that echoes a canned Chat Completions
// response. The handler asserts the request shape is the OpenAI
// Chat Completions API: model + messages[].role/content.
func fakeOpenAI(t *testing.T, wantModel string, captured *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
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
			"id":     "cmpl-test",
			"object": "chat.completion",
			"model":  req.Model,
			"choices": []map[string]any{
				{
					"index":         0,
					"finish_reason": "stop",
					"message":       map[string]string{"role": "assistant", "content": "hi from openai"},
				},
			},
			"usage": map[string]int{"prompt_tokens": 7, "completion_tokens": 9, "total_tokens": 16},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOpenAICompatProvider_CompleteWithUsage(t *testing.T) {
	var captured string
	srv := fakeOpenAI(t, "gpt-4o-mini", &captured)

	p := NewOpenAICompatProvider(OpenAICompatConfig{
		APIKey:    "sk-test",
		BaseURL:   srv.URL,
		ModelName: "gpt-4o-mini",
	})

	text, usage, err := p.CompleteWithUsage(context.Background(), "hi", CompleteOptions{MaxTokens: 32})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if text != "hi from openai" {
		t.Errorf("want text=hi from openai, got %q", text)
	}
	if usage.InputTokens != 7 || usage.OutputTokens != 9 {
		t.Errorf("want usage=7/9, got %d/%d", usage.InputTokens, usage.OutputTokens)
	}
	if !strings.Contains(captured, `"model":"gpt-4o-mini"`) {
		t.Errorf("captured request missing model: %s", captured)
	}
}

func TestOpenAICompatProvider_Available(t *testing.T) {
	// empty key → Available()==false
	p := NewOpenAICompatProvider(OpenAICompatConfig{ModelName: "x"})
	if p.Available() {
		t.Error("want Available()=false with no key")
	}
	p = NewOpenAICompatProvider(OpenAICompatConfig{APIKey: "sk-x", ModelName: "y"})
	if !p.Available() {
		t.Error("want Available()=true with key")
	}
}

func TestOpenAICompatProvider_CompleteWithUsage_Handles4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": "rate limited"}`, http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	p := NewOpenAICompatProvider(OpenAICompatConfig{APIKey: "k", BaseURL: srv.URL, ModelName: "x"})
	_, _, err := p.CompleteWithUsage(context.Background(), "hi", CompleteOptions{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("want error mentioning 429, got %v", err)
	}
}

func TestOpenAICompatProvider_CompleteWithUsage_HandlesMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)
	p := NewOpenAICompatProvider(OpenAICompatConfig{APIKey: "k", BaseURL: srv.URL, ModelName: "x"})
	_, _, err := p.CompleteWithUsage(context.Background(), "hi", CompleteOptions{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("want error mentioning parse, got %v", err)
	}
}

func TestOpenAICompatProvider_CompleteWithUsage_HandlesEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[],"usage":{}}`))
	}))
	t.Cleanup(srv.Close)
	p := NewOpenAICompatProvider(OpenAICompatConfig{APIKey: "k", BaseURL: srv.URL, ModelName: "x"})
	_, _, err := p.CompleteWithUsage(context.Background(), "hi", CompleteOptions{})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "choices") {
		t.Errorf("want error mentioning choices, got %v", err)
	}
}

func TestOpenAICompatProvider_CompleteWithUsage_UnavailableReturnsSentinel(t *testing.T) {
	p := NewOpenAICompatProvider(OpenAICompatConfig{ModelName: "x"}) // no key
	_, _, err := p.CompleteWithUsage(context.Background(), "hi", CompleteOptions{})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Errorf("want errors.Is(err, ErrProviderUnavailable)=true, got %v", err)
	}
}
