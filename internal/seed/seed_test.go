package seed

import (
	"testing"
)

func TestParseRepoURL(t *testing.T) {
	cases := []struct {
		input       string
		wantOwner   string
		wantRepo    string
		wantErr     bool
	}{
		{"https://github.com/XferOps/winnow", "XferOps", "winnow", false},
		{"https://github.com/XferOps/winnow.git", "XferOps", "winnow", false},
		{"http://github.com/XferOps/winnow", "XferOps", "winnow", false},
		{"github.com/XferOps/winnow", "XferOps", "winnow", false},
		{"XferOps/winnow", "XferOps", "winnow", false},
		{"not-a-repo", "", "", true},
		{"", "", "", true},
		{"https://github.com/onlyone", "", "", true},
	}
	for _, tc := range cases {
		owner, repo, err := ParseRepoURL(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseRepoURL(%q) expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRepoURL(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if owner != tc.wantOwner || repo != tc.wantRepo {
			t.Errorf("ParseRepoURL(%q) = (%q, %q), want (%q, %q)",
				tc.input, owner, repo, tc.wantOwner, tc.wantRepo)
		}
	}
}

func TestClassify(t *testing.T) {
	entries := []TreeEntry{
		{Path: "README.md", Type: "blob", Size: 1000},
		{Path: "go.mod", Type: "blob", Size: 500},
		{Path: "cmd/main.go", Type: "blob", Size: 800},
		{Path: "internal/api/router.go", Type: "blob", Size: 2000},
		{Path: "internal/api/auth_handlers.go", Type: "blob", Size: 1500},
		{Path: "internal/models/user.go", Type: "blob", Size: 700},
		{Path: "internal/db/migrations/001_init.sql", Type: "blob", Size: 1200},
		{Path: "Dockerfile", Type: "blob", Size: 300},
		{Path: ".github/workflows/deploy.yml", Type: "blob", Size: 900},
		{Path: "node_modules/something/index.js", Type: "blob", Size: 5000}, // should be skipped
		{Path: "vendor/some/lib.go", Type: "blob", Size: 3000},              // should be skipped
		{Path: "dist/bundle.min.js", Type: "blob", Size: 50000},             // should be skipped
		{Path: "logo.png", Type: "blob", Size: 20000},                       // should be skipped
	}

	selected := Classify(entries)

	// Should not include skipped entries
	for _, f := range selected {
		if f.Path == "node_modules/something/index.js" ||
			f.Path == "vendor/some/lib.go" ||
			f.Path == "dist/bundle.min.js" ||
			f.Path == "logo.png" {
			t.Errorf("Classify should have skipped %q", f.Path)
		}
	}

	// Should classify key files correctly
	found := make(map[string]string) // path -> query_key
	for _, f := range selected {
		found[f.Path] = f.QueryKey
	}

	checks := []struct {
		path    string
		wantQK  string
	}{
		{"README.md", "architecture"},
		{"go.mod", "code-patterns"},
		{"internal/api/router.go", "api-routes"},
		{"internal/api/auth_handlers.go", "auth"},
		{"internal/models/user.go", "domain-model"},
		{"internal/db/migrations/001_init.sql", "database-schema"},
		{"Dockerfile", "deployment"},
		{".github/workflows/deploy.yml", "deployment"},
	}
	for _, c := range checks {
		got, ok := found[c.path]
		if !ok {
			t.Errorf("Classify did not select %q", c.path)
			continue
		}
		if got != c.wantQK {
			t.Errorf("Classify(%q) query_key = %q, want %q", c.path, got, c.wantQK)
		}
	}
}

func TestClassifySkipsOversizedFiles(t *testing.T) {
	entries := []TreeEntry{
		{Path: "README.md", Type: "blob", Size: 200 * 1024}, // 200KB — too large
	}
	selected := Classify(entries)
	if len(selected) != 0 {
		t.Errorf("expected oversized file to be skipped, got %d files", len(selected))
	}
}

func TestCategoriesFromSelected(t *testing.T) {
	files := []SelectedFile{
		{Path: "README.md", QueryKey: "architecture"},
		{Path: "go.mod", QueryKey: "code-patterns"},
		{Path: "router.go", QueryKey: "api-routes"},
	}
	cats := CategoriesFromSelected(files)
	// Should be in priority order: architecture(1), api-routes(3), code-patterns(6)
	if len(cats) != 3 {
		t.Fatalf("expected 3 categories, got %d: %v", len(cats), cats)
	}
	if cats[0] != "architecture" {
		t.Errorf("first category = %q, want architecture", cats[0])
	}
}

func TestShouldSkip(t *testing.T) {
	cases := []struct {
		path string
		skip bool
	}{
		{"node_modules/react/index.js", true},
		{"vendor/github.com/foo/bar.go", true},
		{".git/HEAD", true},
		{"dist/app.js", true},
		{"build/output.js", true},
		{"package-lock.json", true},
		{"go.sum", true},
		{"image.png", true},
		{"font.woff2", true},
		{"internal/api/router.go", false},
		{"README.md", false},
		{"Dockerfile", false},
	}
	for _, tc := range cases {
		got := shouldSkip(tc.path)
		if got != tc.skip {
			t.Errorf("shouldSkip(%q) = %v, want %v", tc.path, got, tc.skip)
		}
	}
}
