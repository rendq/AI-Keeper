package router

import (
	"fmt"
	"net/http"
	"strings"
)

// ProviderType identifies a model provider's API format.
type ProviderType string

const (
	ProviderOpenAI      ProviderType = "openai"
	ProviderAzureOpenAI ProviderType = "azure_openai"
)

// ProviderAdapter translates a generic model call into a provider-specific
// HTTP request. P0 supports OpenAI-compatible and Azure OpenAI formats.
type ProviderAdapter interface {
	// BuildRequest creates an HTTP request for the given endpoint and payload.
	BuildRequest(endpoint RouteEndpoint, payload []byte, apiKey string) (*http.Request, error)
	// ProviderName returns the adapter type identifier.
	ProviderName() ProviderType
}

// NewProviderAdapter returns the appropriate adapter for the given provider.
func NewProviderAdapter(provider string) (ProviderAdapter, error) {
	switch ProviderType(provider) {
	case ProviderOpenAI:
		return &OpenAIAdapter{}, nil
	case ProviderAzureOpenAI:
		return &AzureOpenAIAdapter{}, nil
	default:
		// For other providers, fall back to OpenAI-compatible format.
		return &OpenAIAdapter{}, nil
	}
}

// OpenAIAdapter implements the OpenAI-compatible chat completions format.
type OpenAIAdapter struct{}

func (a *OpenAIAdapter) ProviderName() ProviderType {
	return ProviderOpenAI
}

func (a *OpenAIAdapter) BuildRequest(endpoint RouteEndpoint, payload []byte, apiKey string) (*http.Request, error) {
	url := strings.TrimRight(endpoint.Endpoint, "/") + "/v1/chat/completions"

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	return req, nil
}

// AzureOpenAIAdapter implements the Azure OpenAI format which uses a
// different URL structure and api-key header.
type AzureOpenAIAdapter struct{}

func (a *AzureOpenAIAdapter) ProviderName() ProviderType {
	return ProviderAzureOpenAI
}

func (a *AzureOpenAIAdapter) BuildRequest(endpoint RouteEndpoint, payload []byte, apiKey string) (*http.Request, error) {
	// Azure OpenAI URL format:
	// https://{resource}.openai.azure.com/openai/deployments/{deployment}/chat/completions?api-version=2024-02-01
	url := strings.TrimRight(endpoint.Endpoint, "/")
	if !strings.Contains(url, "chat/completions") {
		url += "/chat/completions?api-version=2024-02-01"
	}

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("azure_openai: build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("api-key", apiKey)
	}

	return req, nil
}
