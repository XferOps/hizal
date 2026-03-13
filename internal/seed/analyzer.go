package seed

import (
	"path/filepath"
	"sort"
	"strings"
)

// Category maps a query_key to its file selection rules and priority.
type Category struct {
	QueryKey    string
	Priority    int // lower = higher priority
	Patterns    []string // path patterns (prefix or suffix match)
	ExactNames  []string // exact filenames
}

var categories = []Category{
	{
		QueryKey: "architecture",
		Priority: 1,
		ExactNames: []string{"README.md", "README.rst", "README.txt", "README", "ARCHITECTURE.md"},
		Patterns:   []string{"docs/", "doc/", "cmd/main.go", "main.go"},
	},
	{
		QueryKey: "domain-model",
		Priority: 2,
		Patterns: []string{"models/", "model/", "entities/", "entity/", "types/", "domain/", "schema/"},
		ExactNames: []string{
			"schema.prisma", "schema.go", "models.go", "types.go",
		},
	},
	{
		QueryKey: "api-routes",
		Priority: 3,
		Patterns: []string{"routes/", "router/", "handlers/", "controllers/", "api/"},
		ExactNames: []string{
			"router.go", "routes.go", "handlers.go",
			"router.ts", "routes.ts",
		},
	},
	{
		QueryKey: "auth",
		Priority: 4,
		Patterns: []string{"auth/", "middleware/"},
		ExactNames: []string{
			"auth.go", "jwt.go", "session.go", "middleware.go",
			"auth.ts", "jwt.ts", "session.ts", "middleware.ts",
		},
	},
	{
		QueryKey: "database-schema",
		Priority: 5,
		Patterns: []string{"migrations/", "migrate/", "db/migrations/", "prisma/"},
		ExactNames: []string{
			"schema.sql", "schema.prisma",
		},
	},
	{
		QueryKey: "code-patterns",
		Priority: 6,
		ExactNames: []string{
			"go.mod", "package.json", "Makefile", "makefile",
			"pyproject.toml", "requirements.txt", "Gemfile",
			"tsconfig.json", ".eslintrc.json", ".eslintrc.js",
		},
	},
	{
		QueryKey: "deployment",
		Priority: 7,
		Patterns: []string{".github/workflows/", "terraform/", "infra/", "infrastructure/", "k8s/", "helm/"},
		ExactNames: []string{
			"Dockerfile", "docker-compose.yml", "docker-compose.yaml",
			"docker-compose.prod.yml", "docker-compose.staging.yml",
			"wrangler.toml", "wrangler.json",
			"fly.toml", "render.yaml", "railway.json",
		},
	},
}

// skip patterns — never include these
var skipPrefixes = []string{
	"vendor/", "node_modules/", ".git/", "dist/", "build/",
	"coverage/", ".next/", ".nuxt/", "out/", "__pycache__/",
}

var skipSuffixes = []string{
	".lock", ".sum", ".min.js", ".min.css", ".map", "-lock.json",
	".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico",
	".woff", ".woff2", ".ttf", ".eot",
	".zip", ".tar", ".gz", ".bin", ".exe",
}

const (
	maxFilesPerCategory = 5
	maxTotalFiles       = 40
	maxFileBytes        = 3000
)

// SelectedFile is a file chosen for a specific category.
type SelectedFile struct {
	Path     string
	QueryKey string
}

// Classify takes a flat list of tree entries and returns the files to fetch,
// grouped by query_key, capped per category and globally.
func Classify(entries []TreeEntry) []SelectedFile {
	// Map query_key -> selected files
	byCat := make(map[string][]SelectedFile)

	for _, entry := range entries {
		if shouldSkip(entry.Path) {
			continue
		}
		// Skip very large files (>100KB) — probably generated/binary
		if entry.Size > 100*1024 {
			continue
		}
		qk := classify(entry.Path)
		if qk == "" {
			continue
		}
		if len(byCat[qk]) < maxFilesPerCategory {
			byCat[qk] = append(byCat[qk], SelectedFile{Path: entry.Path, QueryKey: qk})
		}
	}

	// Flatten, ordered by category priority, up to maxTotalFiles
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Priority < categories[j].Priority
	})

	var result []SelectedFile
	for _, cat := range categories {
		files := byCat[cat.QueryKey]
		for _, f := range files {
			if len(result) >= maxTotalFiles {
				break
			}
			result = append(result, f)
		}
	}
	return result
}

// CategoriesFromSelected returns unique query_keys from a selected file list, in priority order.
func CategoriesFromSelected(files []SelectedFile) []string {
	seen := make(map[string]bool)
	var result []string
	for _, cat := range categories {
		for _, f := range files {
			if f.QueryKey == cat.QueryKey && !seen[cat.QueryKey] {
				seen[cat.QueryKey] = true
				result = append(result, cat.QueryKey)
			}
		}
	}
	return result
}

func shouldSkip(path string) bool {
	for _, p := range skipPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	ext := filepath.Ext(path)
	for _, s := range skipSuffixes {
		if strings.HasSuffix(path, s) || ext == s {
			return true
		}
	}
	return false
}

func classify(path string) string {
	name := filepath.Base(path)
	lower := strings.ToLower(name)

	// Filename-based heuristics run first — more specific than path patterns
	switch {
	case strings.Contains(lower, "auth") || strings.Contains(lower, "jwt") || strings.Contains(lower, "session") || strings.Contains(lower, "middleware"):
		return "auth"
	case strings.Contains(lower, "router") || strings.HasSuffix(lower, "_handler.go") || strings.HasSuffix(lower, "_handlers.go") || strings.HasSuffix(lower, "handler.ts") || strings.HasSuffix(lower, "handlers.ts"):
		return "api-routes"
	case strings.Contains(lower, "model") || strings.Contains(lower, "entity"):
		return "domain-model"
	case strings.HasSuffix(lower, ".sql"):
		return "database-schema"
	}

	for _, cat := range categories {
		// Exact filename match
		for _, n := range cat.ExactNames {
			if strings.EqualFold(name, n) {
				return cat.QueryKey
			}
		}
		// Path prefix/contains match
		for _, p := range cat.Patterns {
			if strings.Contains(path, p) {
				return cat.QueryKey
			}
		}
	}

	return ""
}

// GroupByCategory groups a flat list of SelectedFiles by query_key.
func GroupByCategory(files []SelectedFile) map[string][]SelectedFile {
	m := make(map[string][]SelectedFile)
	for _, f := range files {
		m[f.QueryKey] = append(m[f.QueryKey], f)
	}
	return m
}
