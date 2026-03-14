package seed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/XferOps/winnow/internal/mcp"
	openai "github.com/sashabaranov/go-openai"
)

type fakeContextWriter struct {
	called bool
}

func (f *fakeContextWriter) WriteContext(ctx context.Context, projectID string, in mcp.WriteContextInput) (*mcp.WriteContextResult, error) {
	f.called = true
	return &mcp.WriteContextResult{}, nil
}

func TestProcessCategoryStopsOnContextTimeout(t *testing.T) {
	origFetch := fetchFileContentsFunc
	origGenerate := generateChunkFunc
	t.Cleanup(func() {
		fetchFileContentsFunc = origFetch
		generateChunkFunc = origGenerate
	})

	fetchFileContentsFunc = func(ctx context.Context, meta *RepoMeta, files []string, token string) (map[string]string, error) {
		return map[string]string{"auth.go": "package auth"}, nil
	}
	generateChunkFunc = func(ctx context.Context, llm *openai.Client, meta *RepoMeta, queryKey, description string, contents map[string]string) ([]*generatedChunk, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	writer := &fakeContextWriter{}
	n, err := processCategory(
		ctx,
		writer,
		"project-1",
		&RepoMeta{Owner: "XferOps", Repo: "winnow"},
		"auth",
		"Authentication mechanisms and middleware",
		[]string{"auth.go"},
		"",
		nil,
	)
	if n > 0 {
		t.Fatalf("expected no chunks written on timeout, got %d", n)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if writer.called {
		t.Fatalf("expected WriteContext not to be called after timeout")
	}
}
