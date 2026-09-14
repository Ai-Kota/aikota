package repomap

import (
	"strings"
	"testing"
	"time"
)

// TestBuildOnThisRepository is a smoke test against a real, non-trivial Go
// tree — aikota itself — rather than a fixture. Synthetic tests pin the
// mechanics; this one catches the failure mode they cannot, namely a map that
// builds cleanly but is useless in practice.
//
// Run with -v to see the rendered map.
func TestBuildOnThisRepository(t *testing.T) {
	root := "../.." // internal/repomap -> repo root

	start := time.Now()
	m, err := Build(Options{Root: root, MaxTokens: 1024})
	if err != nil {
		t.Fatalf("Build(%s): %v", root, err)
	}
	elapsed := time.Since(start)

	t.Logf("parsed=%d kept=%d ignored=%d tokens=%d elapsed=%s",
		m.TotalFileCount, m.FileCount, m.IgnoredCount, m.TokenCount, elapsed)

	if m.TotalFileCount < 50 {
		t.Fatalf("parsed only %d files; expected the real tree", m.TotalFileCount)
	}
	if m.FileCount == 0 || m.Rendered == "" {
		t.Fatal("map is empty")
	}
	if m.TokenCount > 1024 {
		t.Errorf("token budget exceeded: %d", m.TokenCount)
	}
	// A large pure-Go repo should parse almost entirely cleanly; a wave of
	// parse failures would mean the extractor is broken, not that the repo is.
	if m.IgnoredCount > m.TotalFileCount/20 {
		t.Errorf("ignored %d of %d files; the extractor looks broken", m.IgnoredCount, m.TotalFileCount)
	}
	// Everything in the map must be a rendered signature block, not a leak.
	if strings.Contains(m.Rendered, "\t") {
		t.Error("map contains tab-indented source; index rendering is leaking")
	}

	t.Logf("rendered map:\n%s", m.Rendered)
}

// TestBuildIsDeterministic: the same tree must rank the same way twice, or the
// agent sees a different codebase on every call.
func TestBuildIsDeterministic(t *testing.T) {
	root := "../.."
	first, err := Build(Options{Root: root, MaxTokens: 2048})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(Options{Root: root, MaxTokens: 2048})
	if err != nil {
		t.Fatal(err)
	}
	if first.Rendered != second.Rendered {
		t.Error("two builds of the same tree produced different maps")
	}
}
