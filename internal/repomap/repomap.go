package repomap

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Options configures one map build.
type Options struct {
	// Root is the repository root to walk. Required.
	Root string
	// MaxTokens bounds the rendered map. Zero uses DefaultMaxTokens.
	MaxTokens int
	// FocusFiles are repo-relative files or directories to boost.
	FocusFiles []string
	// FocusSymbols are symbol names whose defining files to boost.
	FocusSymbols []string
}

// DefaultMaxTokens is the budget when Options.MaxTokens is unset. It matches
// the default peer agents use for a quick overview.
const DefaultMaxTokens = 1024

const (
	// maxFileBytes skips files too large to be hand-written source. Generated
	// files and vendored blobs would otherwise dominate both the scan and the
	// rank.
	maxFileBytes = 1 << 20 // 1 MiB
	// maxScannedFiles bounds the walk so a pathological tree cannot turn one
	// tool call into a multi-minute scan.
	maxScannedFiles = 5000
)

// skipDirs are directory names never worth mapping: version control, vendored
// dependencies, virtualenvs, and build output. This is deliberately a name
// list rather than .gitignore parsing — the goal is to exclude the obvious
// bulk, not to reproduce git's ignore semantics.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "testdata": true,
	"__pycache__": true, ".venv": true, "venv": true, "env": true,
	"dist": true, "build": true, "out": true, "target": true, "obj": true,
	".next": true, ".nuxt": true, ".cache": true, ".idea": true,
	".vscode": true, ".gradle": true, "Pods": true, "DerivedData": true,
}

// Map is a rendered repository map plus the numbers that describe it.
type Map struct {
	Rendered       string
	TokenCount     int
	FileCount      int // files that fit in the budget
	TotalFileCount int // files parsed
	IgnoredCount   int // files skipped as unparsable
	BuildDuration  time.Duration
}

// Build walks opts.Root, parses every Go source file, ranks them and renders a
// token-budgeted map.
//
// A file that fails to parse is counted and skipped rather than failing the
// build: a repository mid-edit is the normal case, and a map of the files that
// do parse is still useful.
func Build(opts Options) (Map, error) {
	started := time.Now()
	budget := opts.MaxTokens
	if budget <= 0 {
		budget = DefaultMaxTokens
	}

	paths, err := walk(opts.Root)
	if err != nil {
		return Map{}, err
	}

	files := make([]fileTags, 0, len(paths))
	ignored := 0
	for _, rel := range paths {
		abs := filepath.Join(opts.Root, filepath.FromSlash(rel))
		src, err := os.ReadFile(abs)
		if err != nil {
			ignored++
			continue
		}
		ft, err := extractGoTags(rel, src)
		if err != nil {
			ignored++
			continue
		}
		files = append(files, ft)
	}

	ranked := buildRanking(files, opts.FocusFiles, opts.FocusSymbols)
	byPath := make(map[string]fileTags, len(files))
	for _, f := range files {
		byPath[f.Path] = f
	}

	rendered, tokens, kept := render(ranked, byPath, budget)
	return Map{
		Rendered:       rendered,
		TokenCount:     tokens,
		FileCount:      kept,
		TotalFileCount: len(files),
		IgnoredCount:   ignored,
		BuildDuration:  time.Since(started),
	}, nil
}

// walk returns repo-relative slash-separated paths of every Go source file
// under root, in a deterministic order.
func walk(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subtree is skipped, not fatal: a map of what we
			// could read beats an error.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if len(out) >= maxScannedFiles {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !isGoSource(rel) {
			return nil
		}
		if info, statErr := d.Info(); statErr == nil && info.Size() > maxFileBytes {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// render emits the budgeted map. A file whose section would exceed the
// remaining budget is skipped, but the scan continues: a smaller, lower-ranked
// file may still fit, and dropping it too would waste the budget.
func render(ranked []rankedFile, byPath map[string]fileTags, maxTokens int) (string, int, int) {
	var b strings.Builder
	tokens := 0
	kept := 0

	for _, r := range ranked {
		ft, ok := byPath[r.Path]
		if !ok {
			continue
		}
		defs := make([]tag, 0, len(ft.Tags))
		for _, t := range ft.Tags {
			if t.Kind == tagDef {
				defs = append(defs, t)
			}
		}
		if len(defs) == 0 {
			continue
		}
		sort.SliceStable(defs, func(i, j int) bool { return defs[i].Line < defs[j].Line })

		section := renderSection(ft.Path, defs)
		cost := estimateTokens(section)
		if tokens+cost > maxTokens {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(section)
		tokens += cost
		kept++
	}
	return b.String(), tokens, kept
}

// renderSection renders one file's signatures with elision markers where
// definitions are not contiguous, so the reader can see that something was
// left out rather than assuming the file is that short.
func renderSection(path string, defs []tag) string {
	var b strings.Builder
	b.WriteString(path)
	b.WriteString(":\n")
	last := 0
	for _, d := range defs {
		if d.Line > last+1 {
			b.WriteString("  ⋮\n")
		}
		b.WriteString("  ")
		b.WriteString(d.Signature)
		b.WriteString("\n")
		last = d.Line
	}
	b.WriteString("  ⋮\n")
	return b.String()
}

// estimateTokens uses the same ceil(chars/4) heuristic the rest of aikota uses
// (internal/compaction), so a budget here means the same thing as a budget
// there.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}
