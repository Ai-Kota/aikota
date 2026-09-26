package dream

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

func TestUCTSearchSelect(t *testing.T) {
	cands := []NearDupPair{
		{A: "a.md", B: "b.md", Similarity: 0.8},
		{A: "c.md", B: "d.md", Similarity: 0.6},
		{A: "e.md", B: "f.md", Similarity: 0.9},
	}
	s := NewUCTSearch(cands, 2)

	// First select should be valid
	idx := s.Select(cands, nil)
	if idx < 0 || idx >= len(cands) {
		t.Errorf("Select() = %d, want valid index", idx)
	}
}

func TestUCTSearchUpdate(t *testing.T) {
	cands := []NearDupPair{
		{A: "a.md", B: "b.md", Similarity: 0.8},
	}
	s := NewUCTSearch(cands, 1)

	s.Update(0, true, 1.0)
	if s.stats[0].Visits != 1 {
		t.Errorf("Visits = %d, want 1", s.stats[0].Visits)
	}
	if s.stats[0].Reward != 1.0 {
		t.Errorf("Reward = %f, want 1.0", s.stats[0].Reward)
	}
}

func TestAdaptiveTreeDepth(t *testing.T) {
	tree := NewAdaptiveTree(3, 10, 1)

	// Small library
	depth := tree.ComputeDepth(50)
	if depth != 3 {
		t.Errorf("depth = %d, want 3", depth)
	}

	// Large library
	depth = tree.ComputeDepth(1500)
	if depth < 3 || depth > 10 {
		t.Errorf("depth = %d, want 3-10", depth)
	}
}

func TestParallelRunner(t *testing.T) {
	cands := []NearDupPair{
		{A: "a.md", B: "b.md", Similarity: 0.8},
		{A: "c.md", B: "d.md", Similarity: 0.6},
	}

	strategies := []SearchStrategy{
		NewUCTSearch(cands, 1),
	}
	runner := NewParallelRunner(strategies, 100*time.Millisecond)

	ctx := context.Background()
	best := runner.Run(ctx, cands)

	// Best decision should have some entries or at least not error
	t.Logf("Best decision score: %.2f, entries: %d", best.Score, len(best.Entries))
}

func TestSortCandidates(t *testing.T) {
	cands := []NearDupPair{
		{A: "a.md", B: "b.md", Similarity: 0.5},
		{A: "c.md", B: "d.md", Similarity: 0.9},
		{A: "e.md", B: "f.md", Similarity: 0.7},
	}
	stats := []PairStats{
		{Visits: 1, Reward: 0.5},
		{Visits: 0, Reward: 0},
		{Visits: 2, Reward: 1.4},
	}

	SortCandidates(cands, stats)

	// Should prioritize unvisited (index 1)
	if cands[0].A != "c.md" {
		t.Errorf("First candidate = %s, want c.md", cands[0].A)
	}
}

func BenchmarkUCTSearchSelect(b *testing.B) {
	cands := make([]NearDupPair, 100)
	for i := range cands {
		cands[i] = NearDupPair{
			A:          "file_a.md",
			B:          "file_b.md",
			Similarity: rand.Float64(),
		}
	}
	s := NewUCTSearch(cands, 5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Select(cands, nil)
	}
}
