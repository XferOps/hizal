package seed

import (
	"path/filepath"
	"strings"
)

// Skip rules — files matching these are never included in the discovery set.
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
	maxFileBytes   = 3000
	maxFilterFiles = 300 // max paths to send to the LLM for category discovery
)

// FilterEntries removes vendor dirs, generated files, binaries, and oversized
// files from the tree, returning a clean flat list for the LLM discovery step.
func FilterEntries(entries []TreeEntry) []TreeEntry {
	result := make([]TreeEntry, 0, len(entries))
	for _, e := range entries {
		if shouldSkip(e.Path) {
			continue
		}
		if e.Size > 100*1024 {
			continue
		}
		result = append(result, e)
		if len(result) >= maxFilterFiles {
			break
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
