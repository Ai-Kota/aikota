// This file implements the MCTS-enhanced search layer for the Dream system.
// It extends the deterministic plan with statistical selection strategies
// (UCT - Upper Confidence Bound for Trees) to optimize memory consolidation.
package dream

import (
	"context"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// SearchStrategy selects which consolidation approach to use for a given
// memory library state. The default is a balanced UCT policy that considers
// both exploitation (current best) and exploration (uncertain candidates).
type SearchStrategy interface {
	// Select returns the index of the candidate pair to evaluate next.
	// Candidates are pairs of near-duplicate files from the plan.
	Select(candidates []NearDupPair, stats []PairStats) int
	// Update records the outcome of evaluating a candidate pair.
	Update(idx int, win bool, reward float64)
}

// PairStats tracks the visit count and total reward for each candidate pair.
type PairStats struct {
	Visits int     // number of times this pair has been evaluated
	Reward float64 // cumulative reward (higher = better merge decision)
}

// UCTSearch implements the Upper Confidence Bound for Trees algorithm.
// It balances exploration (trying uncertain pairs) with exploitation
// (focusing on pairs with high reward).
type UCTSearch struct {
	mu       sync.Mutex
	cands    []NearDupPair
	stats    []PairStats
	epsilon  float64 // exploration rate (0.1 = 10% random moves)
	depth    int     // max tree depth for parallel search
	start    time.Time
}

// NewUCTSearch creates a UCT search strategy with default parameters.
func NewUCTSearch(cands []NearDupPair, depth int) *UCTSearch {
	return &UCTSearch{
		cands:   cands,
		stats:   make([]PairStats, len(cands)),
		epsilon: 0.1,
		depth:   depth,
		start:   time.Now(),
	}
}

// Select implements SearchStrategy. Returns the index of the best candidate
// using UCT formula: Q(s,a) + C * sqrt(ln(N) / n_a)
func (s *UCTSearch) Select(cands []NearDupPair, stats []PairStats) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.cands) == 0 {
		return -1
	}

	// Epsilon-greedy: occasionally pick random for exploration
	if rand.Float64() < s.epsilon {
		return rand.Intn(len(s.cands))
	}

	bestIdx := 0
	bestScore := -math.MaxFloat64


	for i := range s.cands {
		st := s.stats[i] // use i instead of c
		if st.Visits == 0 {
			return i // explore unvisited nodes first
		}
		// UCT score: exploitation + exploration bonus
		qValue := float64(st.Reward) / float64(st.Visits)
		exploration := math.Sqrt(math.Log(float64(i+1)) / float64(st.Visits))
		score := qValue + 0.5*exploration

		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return bestIdx
}

// Update implements SearchStrategy. Records the outcome of evaluating a candidate.
func (s *UCTSearch) Update(idx int, win bool, reward float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if idx < 0 || idx >= len(s.stats) {
		return
	}
	s.stats[idx].Visits++
	s.stats[idx].Reward += reward
}

// ExploreParallel runs parallel exploration of candidate pairs using goroutines.
// Each goroutine evaluates one pair and updates statistics when done.
func (s *UCTSearch) ExploreParallel(ctx context.Context, evaluator PairEvaluator, maxDepth int) map[int]bool {
	results := make(map[int]bool)
	var wg sync.WaitGroup
	limit := min(maxDepth, len(s.cands))

	for i := 0; i < limit; i++ {
		select {
		case <-ctx.Done():
			break
		default:
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				if ctx.Err() != nil {
					return
				}
				win, reward := evaluator(ctx, s.cands[idx])
				s.Update(idx, win, reward)
				results[idx] = win
			}(i)
		}
	}

	wg.Wait()
	return results
}

// PairEvaluator is a function that evaluates a candidate pair and returns
// whether they should be merged (win) and a reward score.
type PairEvaluator func(ctx context.Context, pair NearDupPair) (bool, float64)

// ParallelRunner runs multiple search strategies in parallel and picks the best.
type ParallelRunner struct {
	strategies []SearchStrategy
	timeout    time.Duration
}

// NewParallelRunner creates a runner that evaluates multiple strategies.
func NewParallelRunner(strategies []SearchStrategy, timeout time.Duration) *ParallelRunner {
	return &ParallelRunner{
		strategies: strategies,
		timeout:    timeout,
	}
}

// Run executes all strategies and returns the best result.
func (r *ParallelRunner) Run(ctx context.Context, candidates []NearDupPair) BestDecision {
	type result struct {
		entries []NewEntry
		score   float64
		err     error
	}

	results := make(chan result, len(r.strategies))
	timeout := time.After(r.timeout)

	for _, strat := range r.strategies {
		go func(s SearchStrategy) {
			entries, score, err := r.runStrategy(ctx, s, candidates)
			results <- result{entries: entries, score: score, err: err}
		}(strat)
	}

	best := BestDecision{}
	bestScore := -math.MaxFloat64

	for i := 0; i < len(r.strategies); i++ {
		select {
		case res := <-results:
			if res.err != nil {
				continue
			}
			if res.score > bestScore {
				bestScore = res.score
				best = BestDecision{Entries: res.entries, Score: res.score}
			}
		case <-timeout:
			// Return best found so far
			return best
		}
	}

	return best
}

func (r *ParallelRunner) runStrategy(ctx context.Context, strat SearchStrategy, candidates []NearDupPair) ([]NewEntry, float64, error) {
	// Simulate evaluation with UCT-based selection
	var entries []NewEntry
	score := 0.0

	for i := 0; i < len(candidates); i++ {
		idx := strat.Select(candidates, nil)
		if idx < 0 {
			break
		}
		// In production, this would call the actual evaluator
		// For now, use a simple heuristic
		entries = append(entries, NewEntry{
			Path: candidates[idx].A,
			Body: "consolidated content",
		})
		score += float64(i) / float64(len(candidates))
		strat.Update(idx, true, score)
	}

	return entries, score, nil
}

// BestDecision holds the optimal consolidation result.
type BestDecision struct {
	Entries []NewEntry
	Score   float64
}

// AdaptiveTree implements dynamic depth adjustment based on library state.
type AdaptiveTree struct {
	baseDepth    int
	maxDepth     int
	minDepth     int
	nodeCount    int
	branchingFactor float64
}

// NewAdaptiveTree creates a tree search with adaptive depth.
func NewAdaptiveTree(baseDepth, maxDepth, minDepth int) *AdaptiveTree {
	return &AdaptiveTree{
		baseDepth:     baseDepth,
		maxDepth:      maxDepth,
		minDepth:      minDepth,
		branchingFactor: 2.0,
	}
}

// ComputeDepth calculates the search depth based on library size.
func (t *AdaptiveTree) ComputeDepth(nodeCount int) int {
	t.nodeCount = nodeCount
	// Deeper trees for larger libraries, but bounded
	depth := t.baseDepth
	if nodeCount > 100 {
		depth = min(t.baseDepth+1, t.maxDepth)
	}
	if nodeCount > 1000 {
		depth = min(depth+1, t.maxDepth)
	}
	return max(depth, t.minDepth)
}

// PruningThreshold returns the similarity threshold for pruning low-value branches.
func (t *AdaptiveTree) PruningThreshold() float64 {
	// Higher threshold for larger libraries to reduce search space
	if t.nodeCount > 500 {
		return 0.85
	}
	return 0.7
}

// SortCandidates sorts candidates by priority for search.
func SortCandidates(cands []NearDupPair, stats []PairStats) {
	sort.Slice(cands, func(i, j int) bool {
		// Priority: unvisited > high similarity > recent
		si, sj := stats[i], stats[j]
		if si.Visits == 0 && sj.Visits > 0 {
			return true
		}
		if sj.Visits == 0 && si.Visits > 0 {
			return false
		}
		return cands[i].Similarity > cands[j].Similarity
	})
}

// Min utility
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max utility
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
