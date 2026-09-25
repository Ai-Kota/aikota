// Package memory implements the persistent memory storage layer for aikota.
//
// It is backed by a pure-Go SQLite database (modernc.org/sqlite, no CGO) with
// an FTS5 full-text index over Markdown memory files. This file provides the
// Store type together with database opening and idempotent schema migration.
// Later nodes extend the package with path resolution, reconcile (lazy
// indexing/pruning) and BM25 search.
package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registers the "sqlite" driver
)

// Store is the handle to the memory database. Fields are unexported; later
// nodes access the underlying *sql.DB via DB() and the configured roots via the
// package-internal fields.
type Store struct {
	db     *sql.DB
	root   string // memory root directory (magic layout: global/projects/sessions)
	ccBase string // optional Claude Code base dir for the "cc" scope; "" disables
	ccr    *CCRCache // CCR cache for storing original messages before compaction
}

// Open opens (creating if necessary) the SQLite database at dbPath and runs the
// idempotent schema migration. The parent directory of dbPath is created with
// os.MkdirAll before opening. root is the memory root directory and ccBase is
// the optional Claude Code base directory; both are retained for use by later
// nodes and are not required to exist here.
//
// ccrTtlHours, when >0, enables the CCR cache with the given TTL; otherwise
// CCR remains disabled for this store (use OpenWithCCRTTL to customise).
func Open(dbPath, root, ccBase string, ccrTtlHours int) (*Store, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("memory: empty dbPath")
	}

	// modernc.org/sqlite understands the ":memory:" DSN; only create a parent
	// directory for real file paths.
	if dbPath != ":memory:" {
		if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("memory: create db dir %q: %w", dir, err)
			}
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("memory: open db %q: %w", dbPath, err)
	}

	// A single global DB with a single connection: writes are serialized, which
	// matches the low write frequency and avoids SQLITE_BUSY contention.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: ping db %q: %w", dbPath, err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: migrate: %w", err)
	}

	var ccr *CCRCache
	if ccrTtlHours > 0 {
		ccr = NewCCRCache(db, ccrTtlHours)
	}
	return &Store{db: db, root: root, ccBase: ccBase, ccr: ccr}, nil
}

// OpenDefault opens the memory store with the built-in CCR TTL of 72 hours.
func OpenDefault(dbPath, root, ccBase string) (*Store, error) {
	return Open(dbPath, root, ccBase, 72)
}

// migrate applies the idempotent schema DDL.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(schemaDDL); err != nil {
		return err
	}
	return nil
}

// DB returns the underlying *sql.DB for use by later nodes and tests. It may be
// nil if the store was not successfully opened.
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// Root returns the memory root directory the store was opened with (the magic
// layout root holding global/projects/sessions). It is the canonical source for
// the memory root used by checkpoint persistence and context rebuild
// (<root>/sessions/<id>/checkpoint.md); callers must resolve the memory root
// through this accessor rather than re-deriving it from another store's dir. It
// is "" for a nil store.
func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// CountByScope returns the number of indexed memory entries grouped by scope.
// Scopes with no entries are omitted from the map. It reflects the current
// contents of memory_index; callers that want fresh counts should Reconcile
// first. A nil store or nil db yields an empty map.
func (s *Store) CountByScope() (map[Scope]int, error) {
	out := make(map[Scope]int)
	if s == nil || s.db == nil {
		return out, nil
	}
	rows, err := s.db.Query(`SELECT scope, COUNT(*) FROM memory_index GROUP BY scope`)
	if err != nil {
		return nil, fmt.Errorf("memory: count by scope: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var scope string
		var n int
		if err := rows.Scan(&scope, &n); err != nil {
			return nil, fmt.Errorf("memory: scan scope count: %w", err)
		}
		out[Scope(scope)] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: iterate scope counts: %w", err)
	}
	return out, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// CCRCache returns the CCR cache for storing original messages before compaction.
// Returns nil if the store is nil or CCR was not initialized.
func (s *Store) CCRCache() *CCRCache {
	if s == nil {
		return nil
	}
	return s.ccr
}

// CleanupCCR removes expired entries from the CCR cache and returns the number of
// deleted records. A nil store or uninitialized CCR yields zero with no error.
func (s *Store) CleanupCCR() (int, error) {
	if s == nil || s.ccr == nil {
		return 0, nil
	}
	return s.ccr.Cleanup()
}
