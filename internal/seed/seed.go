package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/XferOps/winnow/internal/mcp"
	openai "github.com/sashabaranov/go-openai"
)

// Event types streamed to the client.
type EventType string

const (
	EventProgress EventType = "progress"
	EventComplete EventType = "complete"
	EventError    EventType = "error"
)

// Event is a single SSE payload.
type Event struct {
	Type EventType
	Data interface{}
}

// ProgressData is sent while seeding is in progress.
type ProgressData struct {
	Step     string `json:"step"`
	Message  string `json:"message"`
	Current  int    `json:"current,omitempty"`
	Total    int    `json:"total,omitempty"`
	Category string `json:"category,omitempty"`
}

// CompleteData is sent when seeding finishes successfully.
type CompleteData struct {
	ChunksWritten int      `json:"chunks_written"`
	Categories    []string `json:"categories"`
}

// ErrorData is sent when seeding fails.
type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Request is the input to Run.
type Request struct {
	RepoURL     string
	GitHubToken string
	ProjectID   string
}

// Run orchestrates the full auto-seed pipeline, emitting Events to the returned channel.
// The channel is closed when seeding completes or errors.
func Run(ctx context.Context, tools *mcp.Tools, req Request) <-chan Event {
	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		run(ctx, tools, req, ch)
	}()
	return ch
}

func emit(ch chan<- Event, t EventType, data interface{}) {
	ch <- Event{Type: t, Data: data}
}

func progress(ch chan<- Event, step, message string, current, total int, category string) {
	emit(ch, EventProgress, ProgressData{
		Step:     step,
		Message:  message,
		Current:  current,
		Total:    total,
		Category: category,
	})
}

func run(ctx context.Context, tools *mcp.Tools, req Request, ch chan<- Event) {
	// 1. Parse repo URL
	owner, repo, err := ParseRepoURL(req.RepoURL)
	if err != nil {
		emit(ch, EventError, ErrorData{Code: "INVALID_URL", Message: err.Error()})
		return
	}

	// 2. Fetch repo metadata
	progress(ch, "fetching_repo", fmt.Sprintf("Connecting to %s/%s...", owner, repo), 0, 0, "")
	meta, err := FetchRepo(ctx, owner, repo, req.GitHubToken)
	if err != nil {
		if ghErr, ok := err.(*GitHubError); ok {
			emit(ch, EventError, ErrorData{Code: string(ghErr.Code), Message: ghErr.Message})
		} else {
			emit(ch, EventError, ErrorData{Code: "FETCH_ERROR", Message: err.Error()})
		}
		return
	}

	// 3. Fetch file tree
	progress(ch, "fetching_tree", "Fetching file tree...", 0, 0, "")
	entries, err := FetchTree(ctx, meta, req.GitHubToken)
	if err != nil {
		if ghErr, ok := err.(*GitHubError); ok {
			emit(ch, EventError, ErrorData{Code: string(ghErr.Code), Message: ghErr.Message})
		} else {
			emit(ch, EventError, ErrorData{Code: "FETCH_ERROR", Message: err.Error()})
		}
		return
	}

	// 4. Classify files
	progress(ch, "classifying", fmt.Sprintf("Scanning %d files...", len(entries)), 0, 0, "")
	selected := Classify(entries)
	if len(selected) == 0 {
		emit(ch, EventError, ErrorData{
			Code:    "NO_FILES_FOUND",
			Message: "No recognisable source files found in this repo. Try the winnow-seed skill for manual seeding.",
		})
		return
	}

	catKeys := CategoriesFromSelected(selected)
	grouped := GroupByCategory(selected)
	progress(ch, "classifying", fmt.Sprintf("Found files across %d categories. Generating context...", len(catKeys)), 0, 0, "")

	// 5. For each category: fetch file contents, call LLM, write chunk
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		emit(ch, EventError, ErrorData{Code: "CONFIG_ERROR", Message: "OPENAI_API_KEY is not configured"})
		return
	}
	llm := openai.NewClient(apiKey)

	written := 0
	writtenCats := make([]string, 0, len(catKeys))

	for i, qk := range catKeys {
		current := i + 1
		total := len(catKeys)

		progress(ch, "generating",
			fmt.Sprintf("Generating %s context... (%d/%d)", qk, current, total),
			current, total, qk,
		)

		files := grouped[qk]
		contents, fetchErr := fetchFileContents(ctx, meta, files, req.GitHubToken)
		if fetchErr != nil {
			// Non-fatal: skip this category if files can't be fetched
			continue
		}
		if len(contents) == 0 {
			continue
		}

		chunk, genErr := generateChunk(ctx, llm, meta, qk, contents)
		if genErr != nil {
			// Non-fatal: skip this category if LLM fails
			continue
		}

		_, writeErr := tools.WriteContext(ctx, req.ProjectID, mcp.WriteContextInput{
			QueryKey: qk,
			Title:    chunk.Title,
			Content:  chunk.Content,
			Gotchas:  chunk.Gotchas,
			Related:  chunk.Related,
		})
		if writeErr != nil {
			continue
		}

		written++
		writtenCats = append(writtenCats, qk)
	}

	emit(ch, EventComplete, CompleteData{
		ChunksWritten: written,
		Categories:    writtenCats,
	})
}

// fetchFileContents reads the content of each selected file, returning path->content pairs.
func fetchFileContents(ctx context.Context, meta *RepoMeta, files []SelectedFile, token string) (map[string]string, error) {
	result := make(map[string]string, len(files))
	for _, f := range files {
		content, err := FetchFile(ctx, meta, f.Path, token, maxFileBytes)
		if err != nil {
			// Skip individual files that can't be fetched (submodules, etc.)
			continue
		}
		result[f.Path] = content
	}
	return result, nil
}

// generatedChunk is the structured output from the LLM.
type generatedChunk struct {
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Gotchas []string `json:"gotchas"`
	Related []string `json:"related"`
}

// categoryGuidance provides category-specific instructions for the LLM.
var categoryGuidance = map[string]string{
	"architecture":    "Focus on the overall system design, tech stack, architectural patterns, and how components fit together.",
	"domain-model":    "Focus on the core data entities, their fields, relationships, and business rules they encode.",
	"api-routes":      "Focus on the HTTP endpoints: methods, paths, request/response shapes, and authentication requirements.",
	"auth":            "Focus on the authentication and authorization mechanisms: how tokens/sessions work, middleware chains, and security patterns.",
	"database-schema": "Focus on the database tables, columns, indexes, constraints, and migration history.",
	"code-patterns":   "Focus on the language/framework conventions, dependency management, coding patterns, and project configuration.",
	"deployment":      "Focus on the infrastructure, CI/CD pipelines, containerisation, and environment configuration.",
}

func generateChunk(ctx context.Context, llm *openai.Client, meta *RepoMeta, queryKey string, contents map[string]string) (*generatedChunk, error) {
	guidance := categoryGuidance[queryKey]
	if guidance == "" {
		guidance = "Focus on what is most important about this part of the codebase."
	}

	// Build the file section of the prompt
	var sb strings.Builder
	for path, content := range contents {
		sb.WriteString(fmt.Sprintf("=== %s ===\n%s\n\n", path, content))
	}

	systemPrompt := `You are a technical documentation writer generating context chunks for an AI knowledge base.

CRITICAL RULES:
- Do not invent, speculate, or fill in gaps. Only document what is clearly present in the provided files.
- Be concise and factual. 2-4 paragraphs of content maximum.
- If the files don't contain enough information for a meaningful chunk, return null.
- Return only valid JSON matching the schema. No markdown fences, no explanation.`

	userPrompt := fmt.Sprintf(`Repository: %s/%s
%s

Category: %s
Guidance: %s

Files:
%s

Return a JSON object with exactly these fields:
{
  "title": "short descriptive title (max 80 chars)",
  "content": "2-4 paragraphs describing what you found",
  "gotchas": ["array of gotchas/warnings, or empty array"],
  "related": ["array of related query_key names from: architecture, domain-model, api-routes, auth, database-schema, code-patterns, deployment — or empty array"]
}

If the files don't contain enough information, return: null`,
		meta.Owner, meta.Repo,
		descriptionLine(meta),
		queryKey,
		guidance,
		sb.String(),
	)

	resp, err := llm.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("LLM returned no choices")
	}

	raw := strings.TrimSpace(resp.Choices[0].Message.Content)
	if raw == "null" || raw == "" {
		return nil, fmt.Errorf("LLM indicated insufficient content for this category")
	}

	var chunk generatedChunk
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		return nil, fmt.Errorf("LLM output was not valid JSON: %w", err)
	}
	if chunk.Title == "" || chunk.Content == "" {
		return nil, fmt.Errorf("LLM returned empty title or content")
	}

	return &chunk, nil
}

func descriptionLine(meta *RepoMeta) string {
	if meta.Description != "" {
		return fmt.Sprintf("Description: %s", meta.Description)
	}
	return ""
}
