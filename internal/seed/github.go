package seed

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ErrorCode represents a specific GitHub API failure the UI can act on.
type ErrorCode string

const (
	ErrRepoNotAccessible ErrorCode = "REPO_NOT_ACCESSIBLE"
	ErrRepoNotFound      ErrorCode = "REPO_NOT_FOUND"
	ErrRepoForbidden     ErrorCode = "REPO_FORBIDDEN"
	ErrRateLimited       ErrorCode = "RATE_LIMITED"
)

// GitHubError is a structured error the handler can surface to the frontend.
type GitHubError struct {
	Code    ErrorCode
	Message string
}

func (e *GitHubError) Error() string { return e.Message }

// RepoMeta holds basic repo information fetched up front.
type RepoMeta struct {
	Owner       string
	Repo        string
	Description string
	Language    string
	DefaultBranch string
}

// TreeEntry represents a single file in the repo tree.
type TreeEntry struct {
	Path string
	Type string // "blob" or "tree"
	Size int
}

// githubClient is a minimal GitHub REST v3 client.
type githubClient struct {
	token  string
	client *http.Client
}

func newGitHubClient(token string) *githubClient {
	return &githubClient{
		token:  token,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *githubClient) get(ctx context.Context, url string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return json.NewDecoder(resp.Body).Decode(out)
	case http.StatusNotFound:
		if c.token == "" {
			return &GitHubError{
				Code:    ErrRepoNotAccessible,
				Message: "This repo isn't accessible. If it's private, provide a GitHub token with 'repo' scope (classic PAT) or 'Contents: Read' (fine-grained PAT).",
			}
		}
		return &GitHubError{
			Code:    ErrRepoNotFound,
			Message: "Repo not found. Check the URL and ensure the token has access.",
		}
	case http.StatusForbidden, http.StatusUnauthorized:
		return &GitHubError{
			Code:    ErrRepoForbidden,
			Message: "Token lacks permission. Ensure it has 'repo' scope (classic PAT) or 'Contents: Read' (fine-grained PAT).",
		}
	case http.StatusTooManyRequests:
		return &GitHubError{
			Code:    ErrRateLimited,
			Message: "GitHub API rate limit hit. Provide a token to increase the limit.",
		}
	default:
		return fmt.Errorf("github API returned status %d", resp.StatusCode)
	}
}

// ParseRepoURL extracts owner and repo from a GitHub URL.
// Accepts https://github.com/owner/repo or owner/repo shorthand.
func ParseRepoURL(raw string) (owner, repo string, err error) {
	raw = strings.TrimSuffix(strings.TrimSpace(raw), ".git")
	raw = strings.TrimPrefix(raw, "https://github.com/")
	raw = strings.TrimPrefix(raw, "http://github.com/")
	raw = strings.TrimPrefix(raw, "github.com/")
	parts := strings.SplitN(raw, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid GitHub repo URL: expected https://github.com/owner/repo")
	}
	return parts[0], parts[1], nil
}

// FetchRepo fetches top-level repo metadata.
func FetchRepo(ctx context.Context, owner, repo, token string) (*RepoMeta, error) {
	c := newGitHubClient(token)
	var raw struct {
		Description   string `json:"description"`
		Language      string `json:"language"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.get(ctx, fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo), &raw); err != nil {
		return nil, err
	}
	return &RepoMeta{
		Owner:         owner,
		Repo:          repo,
		Description:   raw.Description,
		Language:      raw.Language,
		DefaultBranch: raw.DefaultBranch,
	}, nil
}

// FetchTree fetches the full recursive file tree for the default branch.
func FetchTree(ctx context.Context, meta *RepoMeta, token string) ([]TreeEntry, error) {
	c := newGitHubClient(token)
	var raw struct {
		Tree     []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int    `json:"size"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1",
		meta.Owner, meta.Repo, meta.DefaultBranch)
	if err := c.get(ctx, url, &raw); err != nil {
		return nil, err
	}
	entries := make([]TreeEntry, 0, len(raw.Tree))
	for _, e := range raw.Tree {
		if e.Type == "blob" {
			entries = append(entries, TreeEntry{Path: e.Path, Type: e.Type, Size: e.Size})
		}
	}
	return entries, nil
}

// FetchFile fetches the content of a single file (up to maxBytes).
func FetchFile(ctx context.Context, meta *RepoMeta, path, token string, maxBytes int) (string, error) {
	c := newGitHubClient(token)
	var raw struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s",
		meta.Owner, meta.Repo, path)
	if err := c.get(ctx, url, &raw); err != nil {
		return "", err
	}
	if raw.Encoding != "base64" {
		return "", fmt.Errorf("unexpected encoding: %s", raw.Encoding)
	}
	// GitHub base64-encodes with newlines — strip them
	cleaned := strings.ReplaceAll(raw.Content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	content := string(decoded)
	if maxBytes > 0 && len(content) > maxBytes {
		content = content[:maxBytes]
	}
	return content, nil
}
