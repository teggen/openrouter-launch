package openrouter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is the OpenRouter API root used for catalog requests.
// Note this differs from the base URL agents are pointed at.
const DefaultBaseURL = "https://openrouter.ai/api/v1"

// Catalog supplies the model list. Swapping this implementation is the
// single-file change needed to adopt an official SDK later.
type Catalog interface {
	Models(ctx context.Context) ([]Model, error)
}

// Client is an HTTP Catalog. The /models endpoint is public, so no
// credentials are sent.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient returns a Client pointed at the public OpenRouter API.
func NewClient() *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Models fetches the full catalog, most popular first.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	url := c.BaseURL + "/models?sort=most-popular"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "openrouter-launch")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch models: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}
	return DecodeModels(body)
}
