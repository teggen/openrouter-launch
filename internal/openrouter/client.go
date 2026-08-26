package openrouter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

// DefaultBaseURL is the OpenRouter API root used for catalog requests.
// Note this differs from the base URL agents are pointed at.
const DefaultBaseURL = "https://openrouter.ai/api/v1"

// defaultMaxCatalogBytes caps the catalog response read. The real catalog
// measured 0.65 MiB across 413 models on 2026-08-16, so 64 MiB is roughly
// a hundredfold headroom: large enough that catalog growth will never reach
// it, small enough that a runaway response cannot exhaust memory.
const defaultMaxCatalogBytes = 64 << 20

// Client is an HTTP Catalog. The /models endpoint is public, so no
// credentials are sent.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// maxBytes is injectable for tests; 0 means defaultMaxCatalogBytes.
	maxBytes int64
}

func (c *Client) readLimit() int64 {
	if c.maxBytes > 0 {
		return c.maxBytes
	}
	return defaultMaxCatalogBytes
}

// NewClient returns a Client pointed at the public OpenRouter API.
func NewClient() *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Models fetches the full catalog, most popular first. It is what makes
// *Client a catalog.Catalog.
func (c *Client) Models(ctx context.Context) ([]catalog.Model, error) {
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
	// Close's own error is not actionable here: the bytes we care about are
	// read below, and a teardown failure afterwards tells the caller nothing
	// it can act on. The explicit `_ =` marks the choice for errcheck.
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch models: unexpected status %s", resp.Status)
	}

	// Read one byte past the cap so a body sitting exactly ON the cap can be
	// told from one truncated at it: io.LimitReader alone returns a full
	// buffer in both cases, and treating that as an overrun would reject a
	// legitimate catalog of exactly the permitted size.
	//
	// The cap is the only bound on this read that the client controls. The
	// HTTP client's timeout bounds how LONG the transfer may take, not how
	// much it may deliver, so a fast endpoint — misbehaving, or a MITM on a
	// hostile network — is otherwise limited only by bandwidth. Refusing is
	// right where truncating is not: a short read decodes cleanly into a
	// partial catalog, which looks exactly like a legitimately smaller one.
	limit := c.readLimit()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("read models response: response is too large (over %d bytes)", limit)
	}
	return DecodeModels(body)
}
