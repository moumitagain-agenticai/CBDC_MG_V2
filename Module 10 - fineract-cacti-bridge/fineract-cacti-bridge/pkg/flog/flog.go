package flog

import (
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
)

// logDir is where per-source-file log files are written (relative to the
// process working directory).
const logDir = "logs"

// Log is a Logrus logger whose entries are routed to a dedicated file per source
// file, named logs/<tag>.log where <tag> is "<module-number>_<file>".
// Console/structured logging remains the application's primary (zap) logger, so
// Log's own writer is discarded and only the per-file hook persists entries.
var Log = newLogger()

func newLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	l.SetFormatter(&logrus.JSONFormatter{})
	l.SetLevel(logrus.InfoLevel)
	l.AddHook(newFileSplitHook())
	return l
}

// For returns a Logrus entry tagged for a specific source file. The tag must be
// "<module-number>_<file-name>", e.g. "2_bridge".
func For(tag string) *logrus.Entry { return Log.WithField("src", tag) }

// fileSplitHook routes each entry to logs/<src>_Logfile.log based on its "src"
// field, opening and caching one file handle per tag.
type fileSplitHook struct {
	mu    sync.Mutex
	files map[string]*os.File
	fmtr  logrus.Formatter
}

func newFileSplitHook() *fileSplitHook {
	return &fileSplitHook{files: make(map[string]*os.File), fmtr: &logrus.JSONFormatter{}}
}

func (h *fileSplitHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h *fileSplitHook) Fire(e *logrus.Entry) error {
	tag := "app"
	if v, ok := e.Data["src"]; ok {
		if s, ok := v.(string); ok && s != "" {
			tag = s
		}
	}
	f, err := h.fileFor(tag)
	if err != nil {
		return nil // never break the application because logging failed
	}
	line, err := h.fmtr.Format(e)
	if err != nil {
		return nil
	}
	_, _ = f.Write(line)
	return nil
}

func (h *fileSplitHook) fileFor(tag string) (*os.File, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if f, ok := h.files[tag]; ok {
		return f, nil
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(logDir, tag+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	h.files[tag] = f
	return f, nil
}
