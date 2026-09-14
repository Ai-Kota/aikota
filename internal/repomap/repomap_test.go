package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestExtractGoTagsSignatures pins the rendered signature shape: a map is only
// useful if each line reads as a declaration, and a body must never leak in.
func TestExtractGoTagsSignatures(t *testing.T) {
	src := `package sample

import "context"

type Widget struct{ Name string }

type Doer interface{ Do(context.Context) error }

type Alias = string

const MaxWidgets = 10

var Registry map[string]*Widget

func New(name string) *Widget { return &Widget{Name: name} }

func (w *Widget) Do(ctx context.Context) error { return nil }

func Multi(a, b int) (int, error) { return a + b, nil }
`
	ft, err := extractGoTags("sample.go", []byte(src))
	if err != nil {
		t.Fatalf("extractGoTags: %v", err)
	}
	got := map[string]string{}
	for _, tg := range ft.Tags {
		if tg.Kind == tagDef {
			got[tg.Name] = tg.Signature
		}
	}
	want := map[string]string{
		"Widget":    "type Widget struct",
		"Doer":      "type Doer interface",
		"Alias":     "type Alias = string",
		"MaxWidgets": "const MaxWidgets",
		"Registry":  "var Registry",
		"New":       "func New(string) *Widget",
		"Do":        "func (*Widget) Do(context.Context) error",
		"Multi":     "func Multi(int, int) (int, error)",
	}
	for name, wantSig := range want {
		if got[name] != wantSig {
			t.Errorf("signature for %s = %q, want %q", name, got[name], wantSig)
		}
	}
	for _, tg := range ft.Tags {
		if strings.Contains(tg.Signature, "return") || strings.Contains(tg.Signature, "{") {
			t.Errorf("signature %q leaked a body or brace", tg.Signature)
		}
	}
	if !strings.Contains(strings.Join(ft.Imports, ","), "context") {
		t.Errorf("imports = %v, want context", ft.Imports)
	}
}

// TestRankingFavoursDependedUponFiles is the core property: in a synthetic repo
// where many files depend on one, that one must come out on top. Without this
// the ranking is decorative.
func TestRankingFavoursDependedUponFiles(t *testing.T) {
	var files []fileTags
	// core.go defines the symbol everyone else uses.
	core, err := extractGoTags("core.go", []byte(`package p

type Engine struct{}

func NewEngine() *Engine { return &Engine{} }
`))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, core)

	for _, name := range []string{"a.go", "b.go", "c.go"} {
		ft, err := extractGoTags(name, []byte(`package p

func Use`+strings.ToUpper(name[:1])+`() *Engine { return NewEngine() }
`))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, ft)
	}

	ranked := buildRanking(files, nil, nil)
	if len(ranked) == 0 {
		t.Fatal("no ranking produced")
	}
	if ranked[0].Path != "core.go" {
		t.Errorf("top-ranked file = %q, want core.go (every other file depends on it); full order: %v",
			ranked[0].Path, paths(ranked))
	}
}

func paths(rs []rankedFile) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Path
	}
	return out
}

// TestFocusSymbolBoostsItsFile: a focus is a hint about where to look, so it
// must be able to lift a file above the structural winner.
func TestFocusSymbolBoostsItsFile(t *testing.T) {
	files := []fileTags{}
	for _, n := range []string{"alpha", "beta", "gamma"} {
		ft, err := extractGoTags(n+".go", []byte("package p\n\nfunc "+strings.Title(n)+"Run() {}\n"))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, ft)
	}
	// Everyone references Beta, so Beta leads on structure alone.
	hub, _ := extractGoTags("hub.go", []byte("package p\n\nfunc Hub() { BetaRun() }\n"))
	files = append(files, hub)

	plain := buildRanking(files, nil, nil)
	if plain[0].Path == "gamma.go" {
		t.Fatal("precondition failed: gamma should not lead unfocused")
	}

	focused := buildRanking(files, nil, []string{"GammaRun"})
	if focused[0].Path != "gamma.go" {
		t.Errorf("with focus_symbols=[GammaRun], top = %q, want gamma.go; order: %v",
			focused[0].Path, paths(focused))
	}
}

// TestBuildSkipsBulkDirectories: vendor/node_modules would otherwise dominate
// both the scan and the rank.
func TestBuildSkipsBulkDirectories(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, root, "vendor/dep/dep.go", "package dep\n\ntype Huge struct{}\n")
	writeFile(t, root, "node_modules/pkg/index.go", "package pkg\n\nfunc N() {}\n")
	writeFile(t, root, "sub/lib.go", "package sub\n\nfunc L() {}\n")

	m, err := Build(Options{Root: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.TotalFileCount != 2 {
		t.Errorf("parsed %d files, want 2 (main.go + sub/lib.go); map:\n%s", m.TotalFileCount, m.Rendered)
	}
	if strings.Contains(m.Rendered, "vendor/") || strings.Contains(m.Rendered, "node_modules/") {
		t.Errorf("bulk directories leaked into the map:\n%s", m.Rendered)
	}
}

// TestBuildRespectsTokenBudget: the map must fit the budget, and a file too
// large for the remainder must not stop smaller ones from being tried.
func TestBuildRespectsTokenBudget(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "small.go", "package p\n\nfunc Small() {}\n")
	var big strings.Builder
	big.WriteString("package p\n\n")
	for i := 0; i < 200; i++ {
		big.WriteString("func Big")
		big.WriteString(strings.Repeat("X", 30))
		big.WriteString("() {}\n")
	}
	writeFile(t, root, "big.go", big.String())

	for _, budget := range []int{256, 1024} {
		m, err := Build(Options{Root: root, MaxTokens: budget})
		if err != nil {
			t.Fatalf("Build(budget=%d): %v", budget, err)
		}
		if m.TokenCount > budget {
			t.Errorf("budget %d exceeded: used %d", budget, m.TokenCount)
		}
	}
}

// TestBuildSkipsUnparsableFiles: a repo mid-edit must still produce a map.
func TestBuildSkipsUnparsableFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "good.go", "package p\n\nfunc Good() {}\n")
	writeFile(t, root, "broken.go", "package p\n\nfunc Broken( {\n")

	m, err := Build(Options{Root: root})
	if err != nil {
		t.Fatalf("Build with an unparsable file: %v", err)
	}
	if m.IgnoredCount != 1 {
		t.Errorf("IgnoredCount = %d, want 1", m.IgnoredCount)
	}
	if !strings.Contains(m.Rendered, "good.go") {
		t.Errorf("parsable file missing from map:\n%s", m.Rendered)
	}
}
