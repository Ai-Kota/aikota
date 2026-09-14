package repomap

import (
	"math"
	"sort"
	"strings"
)

// Tunables for the ranking, mirroring the parameters used by peer coding
// agents so the output is comparable in character.
const (
	pagerankDamping   = 0.85
	pagerankMaxIter   = 100
	pagerankTolerance = 1e-6

	// maxDefinitionFanout drops symbols defined in more than this many files.
	// A name that ubiquitous carries no structural signal — it is a naming
	// coincidence, not a dependency — and keeping it would connect every file
	// to every other.
	maxDefinitionFanout = 100

	// commonNamePenalty scales the IDF of identifiers that are structurally
	// meaningless (`err`, `ctx`, `data`, `run`). Without it, a repo where every
	// package declares its own `Config` would rank those files as central.
	commonNamePenalty = 0.1

	// importEdgeWeight is the weight of a Go import edge. Imports are an
	// explicit, curated statement of dependency, so they count for more than a
	// single incidental reference.
	importEdgeWeight = 3.0
)

// commonNames are identifiers whose presence says nothing about structure.
// Go-flavoured: the TypeScript original's list, plus the names that recur in
// every Go package (ctx/err/req/resp/opts...) and the stdlib they shadow.
var commonNames = map[string]bool{
	// Generic value names.
	"map": true, "get": true, "set": true, "value": true, "key": true,
	"data": true, "result": true, "error": true, "name": true, "type": true,
	"id": true, "index": true, "item": true, "items": true, "list": true,
	"options": true, "config": true, "args": true, "params": true,
	"props": true, "state": true, "event": true, "callback": true,
	"handler": true, "fn": true, "func": true, "self": true, "this": true,
	"ctx": true, "context": true, "req": true, "res": true, "next": true,
	"err": true, "msg": true, "obj": true, "arr": true, "str": true,
	"num": true, "val": true, "init": true, "start": true, "stop": true,
	"run": true, "main": true, "test": true, "setup": true, "teardown": true,
	"constructor": true, "toString": true, "valueOf": true, "length": true,
	"size": true, "count": true, "push": true, "pop": true, "shift": true,
	"filter": true, "reduce": true, "forEach": true, "find": true,
	"log": true, "warn": true, "info": true, "debug": true, "trace": true,

	// Go-flavoured noise.
	"resp": true, "request": true, "response": true, "opts": true,
	"out": true, "in": true, "buf": true, "b": true, "s": true, "n": true,
	"i": true, "j": true, "ok": true, "nil": true, "true": true,
	"false": true, "string": true, "int": true, "bool": true, "byte": true,
	"float64": true, "any": true, "new": true, "make": true, "len": true,
	"cap": true, "append": true, "copy": true, "delete": true, "close": true,
	"println": true, "print": true, "panic": true, "recover": true,
	"if": true, "else": true, "for": true, "range": true, "return": true,
	"switch": true, "case": true, "default": true, "break": true,
	"continue": true, "go": true, "defer": true, "select": true,
	"chan": true, "interface": true, "struct": true, "package": true,
	"import": true, "const": true, "var": true, "funcType": true,
}

type rankedFile struct {
	Path  string
	Score float64
}

// edgeKey identifies one directed dependency edge (referencing file -> defining
// file) in the weight map.
type edgeKey struct{ from, to string }

// buildRanking turns parsed files into a ranked list.
//
// Edge A→B means "file A depends on file B": A imports B's package, or A
// references a symbol that B defines. The weight of a symbol edge is
// refCount × idf(symbol), so depending on a rare, specific symbol counts for
// more than depending on a ubiquitous one.
func buildRanking(files []fileTags, focusFiles, focusSymbols []string) []rankedFile {
	if len(files) == 0 {
		return nil
	}

	// defIndex: symbol name -> files defining it. Import path -> directory is
	// resolved separately because Go imports name packages, not files.
	defIndex := make(map[string]map[string]bool, len(files))
	dirOf := make(map[string][]string, len(files)) // dir -> files in it
	for i := range files {
		f := &files[i]
		dir := parentDir(f.Path)
		dirOf[dir] = append(dirOf[dir], f.Path)
		for _, t := range f.Tags {
			if t.Kind != tagDef {
				continue
			}
			set := defIndex[t.Name]
			if set == nil {
				set = make(map[string]bool)
				defIndex[t.Name] = set
			}
			set[f.Path] = true
		}
	}

	totalFiles := float64(len(files))

	// idf: log(total / filesDefining), with a penalty for meaningless names.
	// A name defined nowhere in the repo gets the maximum weight; it will not
	// produce an edge anyway, so the value only has to be finite.
	idf := func(name string) float64 {
		docFreq := 1
		if set, ok := defIndex[name]; ok && len(set) > 0 {
			docFreq = len(set)
		}
		v := math.Log(totalFiles / float64(docFreq))
		if commonNames[name] {
			v *= commonNamePenalty
		}
		return v
	}

	weights := make(map[edgeKey]float64)

	for i := range files {
		f := &files[i]
		refCount := make(map[string]int)
		for _, t := range f.Tags {
			if t.Kind == tagRef {
				refCount[t.Name]++
			}
		}
		for name, n := range refCount {
			defs, ok := defIndex[name]
			if !ok || len(defs) > maxDefinitionFanout {
				continue
			}
			w := float64(n) * idf(name)
			if w <= 0 {
				continue
			}
			for defFile := range defs {
				if defFile == f.Path {
					continue // a file does not depend on itself
				}
				weights[edgeKey{f.Path, defFile}] += w
			}
		}

		// Go import edges: an explicit statement of dependency, weighted up.
		for _, imp := range f.Imports {
			for _, target := range filesInImport(dirOf, f.Path, imp) {
				if target == f.Path {
					continue
				}
				weights[edgeKey{f.Path, target}] += importEdgeWeight
			}
		}
	}

	return pageRank(files, weights, focusFiles, focusSymbols, defIndex)
}

// filesInImport maps an import path onto the repo's own files. An import of a
// path outside the module resolves to nothing, which is correct: the map is
// about this repository's internal structure.
func filesInImport(dirOf map[string][]string, fromPath, importPath string) []string {
	// Match the import path against the tail of a directory path. Go import
	// paths are module-prefixed, and the module prefix is not known here, so
	// the suffix match is what carries the information.
	slashed := strings.ReplaceAll(importPath, "\\", "/")
	var out []string
	for dir, fs := range dirOf {
		if dir == "" || dir == "." {
			continue
		}
		if dir == slashed || strings.HasSuffix(slashed, "/"+dir) || strings.HasSuffix(dir, "/"+slashed) {
			out = append(out, fs...)
		}
	}
	return out
}

func parentDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "."
	}
	return p[:i]
}

// pageRank runs weighted PageRank over the dependency graph and applies the
// focus boosts.
func pageRank(files []fileTags, weights map[edgeKey]float64, focusFiles, focusSymbols []string, defIndex map[string]map[string]bool) []rankedFile {
	n := len(files)
	paths := make([]string, n)
	idx := make(map[string]int, n)
	for i := range files {
		paths[i] = files[i].Path
		idx[files[i].Path] = i
	}

	outWeight := make([]float64, n)
	inEdges := make([][]struct {
		from int
		w    float64
	}, n)
	for k, w := range weights {
		fi, okF := idx[k.from]
		ti, okT := idx[k.to]
		if !okF || !okT {
			continue
		}
		outWeight[fi] += w
		inEdges[ti] = append(inEdges[ti], struct {
			from int
			w    float64
		}{fi, w})
	}

	score := make([]float64, n)
	next := make([]float64, n)
	for i := range score {
		score[i] = 1 / float64(n)
	}

	teleport := (1 - pagerankDamping) / float64(n)
	for iter := 0; iter < pagerankMaxIter; iter++ {
		// Dangling mass: nodes with no out-edges would otherwise leak rank.
		dangling := 0.0
		for i := range score {
			if outWeight[i] == 0 {
				dangling += score[i]
			}
		}
		danglingShare := pagerankDamping * dangling / float64(n)

		delta := 0.0
		for i := range next {
			sum := 0.0
			for _, e := range inEdges[i] {
				sum += score[e.from] * e.w / outWeight[e.from]
			}
			next[i] = teleport + danglingShare + pagerankDamping*sum
			delta += math.Abs(next[i] - score[i])
		}
		score, next = next, score
		if delta < pagerankTolerance {
			break
		}
	}

	// Focus boosts, applied after the ranking rather than as personalisation:
	// a focus is a hint about where to look, not a claim about structure.
	boost := make(map[string]float64)
	for _, raw := range focusFiles {
		f := strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/"), "./")
		if f == "" {
			continue
		}
		prefix := strings.TrimSuffix(f, "/") + "/"
		for _, p := range paths {
			if p == f {
				boost[p] = 10
			} else if strings.HasPrefix(p, prefix) {
				boost[p] = 4
			}
		}
	}
	for _, sym := range focusSymbols {
		// defIndex values are sets keyed by file path, so the key is the file.
		for p := range defIndex[strings.TrimSpace(sym)] {
			if boost[p] < 8 {
				boost[p] = 8
			}
		}
	}
	if len(boost) > 0 {
		// Boost neighbours too: a focus file's direct collaborators are usually
		// part of the answer to "where does this live".
		neighbour := make(map[string]bool)
		for k := range weights {
			if _, ok := boost[k.from]; ok {
				neighbour[k.to] = true
			}
			if _, ok := boost[k.to]; ok {
				neighbour[k.from] = true
			}
		}
		for p := range neighbour {
			if boost[p] == 0 {
				boost[p] = 2
			}
		}
	}

	out := make([]rankedFile, 0, n)
	for i, p := range paths {
		s := score[i]
		if b, ok := boost[p]; ok {
			s *= b
		}
		out = append(out, rankedFile{Path: p, Score: s})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Path < out[j].Path // deterministic ties
	})
	return out
}
