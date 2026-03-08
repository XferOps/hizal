package embeddings

import (
	"context"
	"fmt"
	"os"

	openai "github.com/sashabaranov/go-openai"
)

const (
	EmbeddingModel = openai.SmallEmbedding3
	EmbeddingDims  = 1536
)

// Client wraps the OpenAI client for embedding operations.
type Client struct {
	client *openai.Client
}

// NewClient creates an OpenAI embedding client using OPENAI_API_KEY env var.
func NewClient() (*Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}
	return &Client{client: openai.NewClient(apiKey)}, nil
}

// Embed generates a 1536-dimensional embedding for the given text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
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

// EmbedBatch generates embeddings for multiple texts in a single API call.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
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
