package embeddings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const (
	EmbeddingModel = openai.SmallEmbedding3
	EmbeddingDims  = 1536
)

const (
	maxRetries     = 3
	initialBackoff = 500 * time.Millisecond
	maxBackoff     = 4 * time.Second
)

// Client wraps the OpenAI client for embedding operations.
type Client struct {
	client *openai.Client
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		strings.Contains(errStr, "rate_limit_exceeded") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "server_error") ||
		strings.Contains(errStr, "insufficient_quota")
}

// NewClient creates an OpenAI embedding client using OPENAI_API_KEY env var.
func NewClient() (*Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}
	return &Client{client: openai.NewClient(apiKey)}, nil
}

// Embed generates a 1536-dimensional embedding for the given text with retry on failure.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt < maxRetries; attempt++ {
		emb, err := c.embedOnce(ctx, text)
		if err == nil {
			return emb, nil
		}

		lastErr = err

		if !isRetryable(err) {
			return nil, fmt.Errorf("openai embedding request failed (non-retryable): %w", err)
		}

		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("openai embedding request failed: %w", ctx.Err())
			case <-time.After(backoff):
			}
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return nil, fmt.Errorf("openai embedding request failed after %d retries: %w", maxRetries, lastErr)
}

func (c *Client) embedOnce(ctx context.Context, text string) ([]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: []string{text},
		Model: EmbeddingModel,
	})
	if err != nil {
		return nil, fmt.Errorf("openai embedding request failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai returned no embeddings")
	}
	return resp.Data[0].Embedding, nil
}

// EmbedBatch generates embeddings for multiple texts in a single API call with retry on failure.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt < maxRetries; attempt++ {
		results, err := c.embedBatchOnce(ctx, texts)
		if err == nil {
			return results, nil
		}

		lastErr = err

		if !isRetryable(err) {
			return nil, fmt.Errorf("openai batch embedding request failed (non-retryable): %w", err)
		}

		if attempt < maxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("openai batch embedding request failed: %w", ctx.Err())
			case <-time.After(backoff):
			}
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return nil, fmt.Errorf("openai batch embedding request failed after %d retries: %w", maxRetries, lastErr)
}

func (c *Client) embedBatchOnce(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: texts,
		Model: EmbeddingModel,
	})
	if err != nil {
		return nil, fmt.Errorf("openai batch embedding request failed: %w", err)
	}

	results := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		results[i] = d.Embedding
	}
	return results, nil
}
