// Package main implements a mock OpenAI-compatible Chat Completions API server.
// It supports both non-streaming and streaming (SSE) responses with token usage reporting.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// ChatCompletionRequest mirrors the OpenAI chat completion request.
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse mirrors the OpenAI chat completion response.
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a single completion choice.
type Choice struct {
	Index        int      `json:"index"`
	Message      *Message `json:"message,omitempty"`
	Delta        *Message `json:"delta,omitempty"`
	FinishReason *string  `json:"finish_reason"`
}

// Usage reports token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

const mockResponse = "Hello! I am a mock LLM responding to your request."

func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	model := req.Model
	if model == "" {
		model = "mock-gpt-4"
	}

	if req.Stream {
		handleStreaming(w, model)
		return
	}

	handleNonStreaming(w, model)
}

func handleNonStreaming(w http.ResponseWriter, model string) {
	finish := "stop"
	resp := ChatCompletionResponse{
		ID:      "chatcmpl-mock-001",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{
			{
				Index:        0,
				Message:      &Message{Role: "assistant", Content: mockResponse},
				FinishReason: &finish,
			},
		},
		Usage: Usage{
			PromptTokens:     10,
			CompletionTokens: 12,
			TotalTokens:      22,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleStreaming(w http.ResponseWriter, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	tokens := []string{"Hello", "!", " I", " am", " a", " mock", " LLM", " responding", " to", " your", " request", "."}

	for i, token := range tokens {
		chunk := struct {
			ID      string   `json:"id"`
			Object  string   `json:"object"`
			Created int64    `json:"created"`
			Model   string   `json:"model"`
			Choices []Choice `json:"choices"`
		}{
			ID:      "chatcmpl-mock-001",
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []Choice{
				{
					Index: 0,
					Delta: &Message{Role: "", Content: token},
				},
			},
		}

		// Last token gets finish_reason
		if i == len(tokens)-1 {
			finish := "stop"
			chunk.Choices[0].FinishReason = &finish
		}

		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		time.Sleep(50 * time.Millisecond)
	}

	// Final usage chunk (OpenAI style)
	usageChunk := struct {
		ID      string   `json:"id"`
		Object  string   `json:"object"`
		Created int64    `json:"created"`
		Model   string   `json:"model"`
		Choices []Choice `json:"choices"`
		Usage   Usage    `json:"usage"`
	}{
		ID:      "chatcmpl-mock-001",
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{},
		Usage: Usage{
			PromptTokens:     10,
			CompletionTokens: len(tokens),
			TotalTokens:      10 + len(tokens),
		},
	}
	data, _ := json.Marshal(usageChunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleModels(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data": []map[string]interface{}{
			{"id": "mock-gpt-4", "object": "model", "owned_by": "mock"},
		},
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handleChatCompletions)
	mux.HandleFunc("/v1/models", handleModels)
	mux.HandleFunc("/healthz", handleHealth)

	log.Printf("mock-llm listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
