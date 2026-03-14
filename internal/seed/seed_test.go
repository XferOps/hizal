package seed

import (
	"testing"
)

func TestParseRepoURL(t *testing.T) {
	cases := []struct {
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
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

func TestFilterEntries(t *testing.T) {
	entries := []TreeEntry{
		{Path: "README.md", Type: "blob", Size: 1000},
		{Path: "go.mod", Type: "blob", Size: 500},
		{Path: "internal/api/router.go", Type: "blob", Size: 2000},
		{Path: "internal/api/auth_handlers.go", Type: "blob", Size: 1500},
		{Path: "internal/models/user.go", Type: "blob", Size: 700},
		{Path: "internal/db/migrations/001_init.sql", Type: "blob", Size: 1200},
		{Path: "Dockerfile", Type: "blob", Size: 300},
		// should be skipped
		{Path: "node_modules/something/index.js", Type: "blob", Size: 5000},
		{Path: "vendor/some/lib.go", Type: "blob", Size: 3000},
		{Path: "dist/bundle.min.js", Type: "blob", Size: 50000},
		{Path: "logo.png", Type: "blob", Size: 20000},
		{Path: "package-lock.json", Type: "blob", Size: 80000},
		{Path: "go.sum", Type: "blob", Size: 10000},
	}

	filtered := FilterEntries(entries)

	skipped := map[string]bool{
		"node_modules/something/index.js": true,
		"vendor/some/lib.go":              true,
		"dist/bundle.min.js":              true,
		"logo.png":                        true,
		"package-lock.json":               true,
		"go.sum":                          true,
	}
	for _, f := range filtered {
		if skipped[f.Path] {
			t.Errorf("FilterEntries should have skipped %q", f.Path)
		}
	}

	// All expected files should be present
	kept := map[string]bool{}
	for _, f := range filtered {
		kept[f.Path] = true
	}
	for _, want := range []string{"README.md", "go.mod", "internal/api/router.go", "Dockerfile"} {
		if !kept[want] {
			t.Errorf("FilterEntries dropped %q unexpectedly", want)
		}
	}
}

func TestFilterEntriesSkipsOversizedFiles(t *testing.T) {
	entries := []TreeEntry{
		{Path: "README.md", Type: "blob", Size: 200 * 1024}, // 200KB — too large
		{Path: "go.mod", Type: "blob", Size: 500},
	}
	filtered := FilterEntries(entries)
	if len(filtered) != 1 || filtered[0].Path != "go.mod" {
		t.Errorf("expected only go.mod, got %v", filtered)
	}
}

func TestFilterEntriesRespectsMaxCap(t *testing.T) {
	entries := make([]TreeEntry, maxFilterFiles+50)
	for i := range entries {
		entries[i] = TreeEntry{Path: "src/file.go", Type: "blob", Size: 100}
	}
	filtered := FilterEntries(entries)
	if len(filtered) > maxFilterFiles {
		t.Errorf("FilterEntries returned %d entries, want <= %d", len(filtered), maxFilterFiles)
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
