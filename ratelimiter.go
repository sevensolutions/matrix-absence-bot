package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"maunium.net/go/mautrix/id"
)

// RateLimiter tracks the last calendar day (in local time, "YYYY-MM-DD")
// each sender received an auto-reply, so nobody gets more than one reply
// per day. State is persisted to a JSON file so the "once per day" limit
// survives bot restarts within the same day.
type RateLimiter struct {
	path string

	mu       sync.Mutex
	lastSent map[id.UserID]string
}

func NewRateLimiter(path string) (*RateLimiter, error) {
	rl := &RateLimiter{
		path:     path,
		lastSent: make(map[id.UserID]string),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return rl, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return rl, nil
	}
	if err := json.Unmarshal(data, &rl.lastSent); err != nil {
		return nil, err
	}
	return rl, nil
}

// today returns the current local calendar day as "YYYY-MM-DD".
func today() string {
	return time.Now().Format("2006-01-02")
}

// ShouldReply reports whether sender has not already received an auto-reply
// today.
func (rl *RateLimiter) ShouldReply(sender id.UserID) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.lastSent[sender] != today()
}

// MarkReplied records that sender was just replied to today, and persists
// the updated state to disk.
func (rl *RateLimiter) MarkReplied(sender id.UserID) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.lastSent[sender] = today()
	return rl.persistLocked()
}

// persistLocked writes the state atomically (write to a temp file, then
// rename) so a crash mid-write can't corrupt the state file.
func (rl *RateLimiter) persistLocked() error {
	data, err := json.MarshalIndent(rl.lastSent, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(rl.path)
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, rl.path)
}
