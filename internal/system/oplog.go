package system

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// DefaultLogPath is the operation log written by the real Runner.
const DefaultLogPath = "/var/log/litenode.log"

// DefaultLogMaxSize is the size at which the log is rotated to <path>.1.
const DefaultLogMaxSize = 1 << 20

// OpLog appends one line per mutating operation to a log file. When the file
// exceeds MaxSize it is renamed to <Path>.1 (replacing any older copy), so the
// log never uses more than about twice MaxSize.
//
// Logging is best effort: a failure to write the log never fails the
// operation being logged.
type OpLog struct {
	Path    string
	MaxSize int64
	// Now returns the current time. It is replaceable in tests.
	Now func() time.Time

	mu sync.Mutex
}

// NewOpLog returns an OpLog writing to path with the default size limit.
func NewOpLog(path string) *OpLog {
	return &OpLog{Path: path, MaxSize: DefaultLogMaxSize, Now: time.Now}
}

// Record logs an operation and its outcome. It is a no-op on a nil OpLog.
func (l *OpLog) Record(op string, err error) {
	if l == nil {
		return
	}
	outcome := "ok"
	if err != nil {
		outcome = "error: " + err.Error()
	}
	line := fmt.Sprintf("%s %s: %s", l.Now().Format(time.RFC3339), op, outcome)
	// Keep one entry per line even if an error message spans several lines.
	line = strings.ReplaceAll(line, "\n", `\n`) + "\n"

	l.mu.Lock()
	defer l.mu.Unlock()
	if fi, err := os.Stat(l.Path); err == nil && fi.Size() >= l.MaxSize {
		os.Rename(l.Path, l.Path+".1")
	}
	f, err := os.OpenFile(l.Path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line)
}
