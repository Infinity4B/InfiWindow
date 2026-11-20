// Package activity is the log shown in the window: every feature writes the
// things it did there, newest last.
package activity

import (
	"fmt"
	"sync"
	"time"
)

// Level of an entry.
type Level int

const (
	Info Level = iota
	Good
	Warn
	Bad
)

// Entry is one line of the log.
type Entry struct {
	At     time.Time
	Level  Level
	Source string // the feature that wrote it, shown as a tag
	Text   string
}

// Max is how many entries the log keeps.
const Max = 300

// Log is safe for concurrent use.
type Log struct {
	mu       sync.Mutex
	entries  []Entry
	now      func() time.Time
	onChange func()
}

// New makes a log; onChange (may be nil) runs after every change, outside
// the lock, on the goroutine that made it.
func New(onChange func()) *Log { return &Log{now: time.Now, onChange: onChange} }

// SetClock replaces time.Now, for tests.
func (l *Log) SetClock(now func() time.Time) { l.mu.Lock(); l.now = now; l.mu.Unlock() }

// Add appends an entry.
func (l *Log) Add(source string, level Level, format string, args ...any) {
	l.mu.Lock()
	l.entries = append(l.entries, Entry{At: l.now(), Level: level, Source: source, Text: fmt.Sprintf(format, args...)})
	if len(l.entries) > Max {
		l.entries = l.entries[len(l.entries)-Max:]
	}
	fn := l.onChange
	l.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Entries returns a copy of the log, oldest first.
func (l *Log) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Entry(nil), l.entries...)
}

// Clear empties the log.
func (l *Log) Clear() {
	l.mu.Lock()
	l.entries = nil
	fn := l.onChange
	l.mu.Unlock()
	if fn != nil {
		fn()
	}
}
