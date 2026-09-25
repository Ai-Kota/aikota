package memory

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CCRCache stores compressed content's original text for on-demand retrieval.
// This enables "reversible compression" — when compaction discards history,
// the original text can still be retrieved via memory_retrieve tool.
//
// The cache uses TTL-based eviction: entries older than ttlHours are cleaned
// up during routine operations. Default TTL is 72 hours.
type CCRCache struct {
	db       *sql.DB
	ttlHours int
}

// CCREntry represents a cached original message with its compressed form.
type CCREntry struct {
	// ID is the unique identifier for this entry.
	ID string
	// Original is the full text of the message before compression.
	Original string
	// Compressed is the summary/compact form of the message.
	Compressed string
	// CreatedAt is when this entry was cached.
	CreatedAt time.Time
	// TTLHours is how long to keep this entry (default 72).
	TTLHours int
}

// NewCCRCache creates a new CCR cache backed by the given database.
func NewCCRCache(db *sql.DB, ttlHours int) *CCRCache {
	if ttlHours <= 0 {
		ttlHours = 72 // default 72 hours
	}
	return &CCRCache{
		db:       db,
		ttlHours: ttlHours,
	}
}

// contentHash returns a deterministic hash of the given text.
func contentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Store caches the original text for a message that has been compressed.
// Returns the cache ID for later retrieval.
//
// If the original text is empty or identical to the compressed form, no
// entry is stored (optimization).
func (c *CCRCache) Store(original, compressed string) (string, error) {
	if original == "" || original == compressed {
		return "", nil
	}

	// Check if we already have this content (by content hash)
	hash := contentHash(original)

	var existingID string
	err := c.db.QueryRow(
		`SELECT id FROM memory_ccr WHERE content_hash = ? LIMIT 1`,
		hash,
	).Scan(&existingID)
	if err == nil && existingID != "" {
		// Already cached, return existing ID
		return existingID, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return "", fmt.Errorf("ccr: check existing: %w", err)
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	_, err = c.db.Exec(
		`INSERT INTO memory_ccr (id, content_hash, original, compressed, created_at, ttl_hours)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, hash, original, compressed, now, c.ttlHours,
	)
	if err != nil {
		return "", fmt.Errorf("ccr: insert: %w", err)
	}

	return id, nil
}

// Retrieve fetches the original text by cache ID.
// Returns (text, true) if found and not expired, (empty, false) otherwise.
func (c *CCRCache) Retrieve(id string) (string, bool) {
	var original string
	var createdAt time.Time
	var ttlHours int

	err := c.db.QueryRow(
		`SELECT original, created_at, ttl_hours FROM memory_ccr WHERE id = ?`,
		id,
	).Scan(&original, &createdAt, &ttlHours)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false
		}
		return "", false
	}

	// Check expiration
	expiry := createdAt.Add(time.Duration(ttlHours) * time.Hour)
	if time.Now().UTC().After(expiry) {
		// Expired, clean up
		_ = c.Delete(id)
		return "", false
	}

	return original, true
}

// Delete removes a cached entry.
func (c *CCRCache) Delete(id string) error {
	_, err := c.db.Exec(`DELETE FROM memory_ccr WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("ccr: delete: %w", err)
	}
	return nil
}

// Cleanup removes all expired entries and returns the count removed.
func (c *CCRCache) Cleanup() (int, error) {
	now := time.Now().UTC()
	result, err := c.db.Exec(
		`DELETE FROM memory_ccr WHERE created_at < ?`,
		now.Add(-time.Duration(c.ttlHours)*time.Hour),
	)
	if err != nil {
		return 0, fmt.Errorf("ccr: cleanup: %w", err)
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// List returns all active cache entries (non-expired).
func (c *CCRCache) List() ([]CCREntry, error) {
	rows, err := c.db.Query(
		`SELECT id, original, compressed, created_at, ttl_hours
		 FROM memory_ccr
		 WHERE created_at > datetime('now', ?)
		 ORDER BY created_at DESC`,
		fmt.Sprintf("-%d hours", c.ttlHours),
	)
	if err != nil {
		return nil, fmt.Errorf("ccr: list: %w", err)
	}
	defer rows.Close()

	var entries []CCREntry
	for rows.Next() {
		var e CCREntry
		if err := rows.Scan(&e.ID, &e.Original, &e.Compressed, &e.CreatedAt, &e.TTLHours); err != nil {
			return nil, fmt.Errorf("ccr: scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Stats returns cache statistics.
func (c *CCRCache) Stats() (total, active int, err error) {
	err = c.db.QueryRow(`SELECT COUNT(*) FROM memory_ccr`).Scan(&total)
	if err != nil {
		return 0, 0, err
	}
	err = c.db.QueryRow(
		`SELECT COUNT(*) FROM memory_ccr WHERE created_at > datetime('now', ?)`,
		fmt.Sprintf("-%d hours", c.ttlHours),
	).Scan(&active)
	if err != nil {
		return 0, 0, err
	}
	return total, active, nil
}
