// This file implements background bash execution: a BashJobStore holding the
// commands launched with run_in_background, and the BashJob state each one
// carries. A background job is detached from the turn context (which is canceled
// when the turn ends) and runs under its own cancelable context until it exits or
// kill_bash stops it. Its combined stdout/stderr accumulates in a buffer that
// bash_output drains incrementally, mirroring Claude Code's background shells +
// BashOutput/KillShell.
package agenttool

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"
)

// BashJobStatus is the lifecycle state of a background bash job.
type BashJobStatus string

const (
	// BashRunning means the command is still executing.
	BashRunning BashJobStatus = "running"
	// BashExited means the command finished (successfully or not) or was killed.
	BashExited BashJobStatus = "exited"
)

// BashJob is a single background command: its identity, growing combined output,
// and terminal state. All fields are guarded by mu so the running command's
// writer, bash_output reads, and kill_bash can touch it concurrently.
//
// The buffer is capped at bashMaxBackgroundBufBytes to prevent unbounded memory
// growth from a misbehaving or long-running command. When the cap is hit, new
// bytes are dropped and a watermark is written so bash_output still reports
// progress rather than silently losing data.
type BashJob struct {
	// ID is the stable handle (e.g. "bash_1") bash_output/kill_bash address.
	ID string
	// Command is the shell command line, kept for listing/display.
	Command string
	// StartedAt is when the command was launched.
	StartedAt time.Time

	mu       sync.Mutex
	buf      bytes.Buffer
	capHit   bool // true once the buffer cap has been hit
	cursor   int // bytes of buf already returned by bash_output
	status   BashJobStatus
	exitCode int
	errMsg   string
	finished time.Time
	cancel   context.CancelFunc
}

// bashMaxBackgroundBufBytes is the hard cap on a single background job's
// stdout/stderr buffer. A dev server running for hours can emit gigabytes; we
// keep the last 2 MiB and drop the rest so a single hung job cannot exhaust
// the host's memory.
const bashMaxBackgroundBufBytes = 2 * 1024 * 1024

// jobWriter adapts a BashJob to io.Writer so it can be a command's Stdout/Stderr;
// each write appends to the job's combined buffer under its lock.
type jobWriter struct{ job *BashJob }

func (w jobWriter) Write(p []byte) (int, error) {
	w.job.mu.Lock()
	defer w.job.mu.Unlock()
	if w.job.capHit {
		// Already over cap: drop everything but keep the watermark.
		return len(p), nil
	}
	w.job.buf.Write(p)
	if w.job.buf.Len() > bashMaxBackgroundBufBytes {
		// Cap hit: truncate head, keep tail, write watermark.
		data := w.job.buf.Bytes()
		keep := data[len(data)-bashMaxBackgroundBufBytes:]
		w.job.buf.Reset()
		w.job.buf.Write(keep)
		w.job.buf.WriteString("\n[cap hit: dropped " + fmt.Sprintf("%d", len(data)-len(keep)) + " bytes, keeping tail]")
		w.job.capHit = true
	}
	return len(p), nil
}

// writer returns an io.Writer that appends to the job's output buffer.
func (j *BashJob) writer() jobWriter { return jobWriter{job: j} }

// finish records the command's terminal state from the error returned by
// cmd.Wait (nil = success). It is idempotent-safe to call once per job.
func (j *BashJob) finish(exitCode int, errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = BashExited
	j.exitCode = exitCode
	j.errMsg = errMsg
	j.finished = time.Now()
}

// kill cancels the job's context (terminating the process) and marks it exited if
// it was still running. It reports whether the job was running when called.
func (j *BashJob) kill() bool {
	j.mu.Lock()
	running := j.status == BashRunning
	j.mu.Unlock()
	if j.cancel != nil {
		j.cancel()
	}
	return running
}

// readNew returns the output accumulated since the last read and advances the
// cursor, so successive bash_output calls stream the command's output without
// repeating what was already seen.
func (j *BashJob) readNew() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	all := j.buf.Bytes()
	if j.cursor > len(all) {
		j.cursor = len(all)
	}
	out := string(all[j.cursor:])
	j.cursor = len(all)
	return out
}

// snapshot returns the job's current status fields for reporting without exposing
// the mutex-guarded internals.
func (j *BashJob) snapshot() (status BashJobStatus, exitCode int, errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status, j.exitCode, j.errMsg
}

// capReached returns whether the buffer cap has been hit.
func (j *BashJob) capReached() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.capHit
}

// BashJobStore holds a session's background bash jobs. A single store is shared by
// the bash, bash_output and kill_bash tools so a job launched by one is visible to
// the others. It is safe for concurrent use.
type BashJobStore struct {
	mu   sync.Mutex
	jobs map[string]*BashJob
	seq  int
}

// NewBashJobStore returns an empty store.
func NewBashJobStore() *BashJobStore {
	return &BashJobStore{jobs: map[string]*BashJob{}}
}

// create registers a new running job for command with its cancel func, assigning
// a readable sequential id.
func (s *BashJobStore) create(command string, cancel context.CancelFunc) *BashJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	job := &BashJob{
		ID:        fmt.Sprintf("bash_%d", s.seq),
		Command:   command,
		StartedAt: time.Now(),
		status:    BashRunning,
		cancel:    cancel,
	}
	s.jobs[job.ID] = job
	return job
}

// Get returns the job with the given id, or (nil, false).
func (s *BashJobStore) Get(id string) (*BashJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	return j, ok
}

// List returns the jobs in creation order.
func (s *BashJobStore) List() []*BashJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*BashJob, 0, len(s.jobs))
	for i := 1; i <= s.seq; i++ {
		if j, ok := s.jobs[fmt.Sprintf("bash_%d", i)]; ok {
			out = append(out, j)
		}
	}
	return out
}

// KillAll cancels every still-running job. It is intended for session shutdown so
// background processes are not orphaned.
func (s *BashJobStore) KillAll() {
	for _, j := range s.List() {
		j.kill()
	}
}
