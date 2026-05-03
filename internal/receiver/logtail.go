package receiver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

// LogTailOptions configures a LogTailReceiver. Exactly one of Path / UseStdin
// / Listen must be set. Format may be "json", "logfmt", "plain", or ""
// (auto-detect on the first non-empty line).
type LogTailOptions struct {
	Path     string
	UseStdin bool
	Listen   string
	Format   string
	Service  string
}

// LogTailReceiver tails files, stdin, or TCP streams and emits model.LogEvent.
type LogTailReceiver struct {
	name string
	opts LogTailOptions

	mu      sync.Mutex
	cancel  context.CancelFunc
	listener net.Listener
	out     chan<- any
	stopped chan struct{}
}

func NewLogTail(name string, opts LogTailOptions) *LogTailReceiver {
	if opts.Service == "" {
		opts.Service = name
	}
	return &LogTailReceiver{name: name, opts: opts}
}

func (r *LogTailReceiver) Name() string { return r.name }

func (r *LogTailReceiver) Start(ctx context.Context, out chan<- any) error {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return errors.New("logtail: already started")
	}
	subCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.out = out
	r.stopped = make(chan struct{})
	r.mu.Unlock()

	switch {
	case r.opts.Listen != "":
		lis, err := net.Listen("tcp", r.opts.Listen)
		if err != nil {
			cancel()
			return fmt.Errorf("logtail listen %s: %w", r.opts.Listen, err)
		}
		r.mu.Lock()
		r.listener = lis
		r.mu.Unlock()
		go r.runTCP(subCtx, lis)
	case r.opts.UseStdin:
		go r.runReader(subCtx, os.Stdin)
	case r.opts.Path != "":
		go r.runFile(subCtx, r.opts.Path)
	default:
		cancel()
		return errors.New("logtail: must specify Path, UseStdin, or Listen")
	}
	return nil
}

func (r *LogTailReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if r.listener != nil {
		_ = r.listener.Close()
		r.listener = nil
	}
	return nil
}

func (r *LogTailReceiver) emit(ev any) {
	r.mu.Lock()
	out := r.out
	r.mu.Unlock()
	if out == nil {
		return
	}
	select {
	case out <- ev:
	default:
	}
}

// ── file tail with rotation detection ──────────────────────────────────────

func (r *LogTailReceiver) runFile(ctx context.Context, path string) {
	parser := newLineParser(r.opts.Format)
	var (
		f       *os.File
		fi      os.FileInfo
		reader  *bufio.Reader
	)
	open := func() error {
		nf, err := os.Open(path)
		if err != nil {
			return err
		}
		// Seek to end so we don't replay history.
		if _, err := nf.Seek(0, io.SeekEnd); err != nil {
			_ = nf.Close()
			return err
		}
		ni, err := nf.Stat()
		if err != nil {
			_ = nf.Close()
			return err
		}
		if f != nil {
			_ = f.Close()
		}
		f, fi, reader = nf, ni, bufio.NewReader(nf)
		return nil
	}
	if err := open(); err != nil {
		// File may not exist yet; retry until ctx is done.
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := open(); err == nil {
					goto loop
				}
			}
		}
	}
loop:
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			r.emitLine(parser, strings.TrimRight(line, "\r\n"))
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return
			}
			// EOF: check rotation, then sleep briefly.
			if cur, statErr := os.Stat(path); statErr == nil && !os.SameFile(fi, cur) {
				if err := open(); err != nil {
					select {
					case <-ctx.Done():
						return
					case <-time.After(500 * time.Millisecond):
						continue
					}
				}
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

// ── stdin / generic io.Reader ──────────────────────────────────────────────

func (r *LogTailReceiver) runReader(ctx context.Context, src io.Reader) {
	parser := newLineParser(r.opts.Format)
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				return
			}
			r.emitLine(parser, line)
		}
	}
}

// ── TCP listener ───────────────────────────────────────────────────────────

func (r *LogTailReceiver) runTCP(ctx context.Context, lis net.Listener) {
	go func() {
		<-ctx.Done()
		_ = lis.Close()
	}()
	for {
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		go r.runReader(ctx, conn)
	}
}

func (r *LogTailReceiver) emitLine(p *lineParser, line string) {
	if line == "" {
		return
	}
	ev := p.parse(line, r.opts.Service)
	r.emit(ev)
}

// ── parsing ────────────────────────────────────────────────────────────────

type lineParser struct {
	mode string
}

func newLineParser(format string) *lineParser {
	return &lineParser{mode: strings.ToLower(format)}
}

func (p *lineParser) parse(line, service string) model.LogEvent {
	mode := p.mode
	if mode == "" || mode == "auto" {
		mode = detectFormat(line)
	}
	switch mode {
	case "json":
		if ev, ok := parseJSON([]byte(line), service); ok {
			return ev
		}
	case "logfmt":
		if ev, ok := parseLogfmt(line, service); ok {
			return ev
		}
	}
	return parsePlain(line, service)
}

func detectFormat(line string) string {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}") {
		return "json"
	}
	if strings.Contains(t, "=") && !strings.Contains(t, ": ") {
		// rough heuristic: contains key=value pairs
		return "logfmt"
	}
	return "plain"
}

var levelRe = regexp.MustCompile(`(?i)\[(debug|info|warn|warning|error|err)\]`)

func parsePlain(line, service string) model.LogEvent {
	ev := model.LogEvent{
		Timestamp: time.Now(),
		Service:   service,
		Level:     model.LevelInfo,
		Body:      line,
	}
	if m := levelRe.FindStringSubmatch(line); len(m) > 1 {
		ev.Level = normalizeLevel(m[1])
	}
	return ev
}

func parseJSON(data []byte, service string) (model.LogEvent, bool) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return model.LogEvent{}, false
	}
	ev := model.LogEvent{Service: service, Attrs: map[string]string{}}
	if v, ok := pickFirst(m, "level", "severity", "lvl"); ok {
		ev.Level = normalizeLevel(stringify(v))
	}
	if v, ok := pickFirst(m, "msg", "message", "body"); ok {
		ev.Body = stringify(v)
	}
	if v, ok := pickFirst(m, "time", "timestamp", "ts", "@timestamp"); ok {
		ev.Timestamp = parseTime(stringify(v))
	}
	if v, ok := pickFirst(m, "trace_id", "traceId", "trace"); ok {
		ev.TraceID = stringify(v)
	}
	if v, ok := pickFirst(m, "service", "service.name"); ok {
		ev.Service = stringify(v)
	}
	for k, v := range m {
		ev.Attrs[k] = stringify(v)
	}
	if ev.Level == "" {
		ev.Level = model.LevelInfo
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	if ev.Body == "" {
		ev.Body = string(data)
	}
	return ev, true
}

func pickFirst(m map[string]any, keys ...string) (any, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			delete(m, k)
			return v, true
		}
	}
	return nil, false
}

func parseLogfmt(line, service string) (model.LogEvent, bool) {
	pairs, ok := parseLogfmtPairs(line)
	if !ok || len(pairs) == 0 {
		return model.LogEvent{}, false
	}
	ev := model.LogEvent{Service: service, Attrs: map[string]string{}}
	for k, v := range pairs {
		switch strings.ToLower(k) {
		case "level", "severity", "lvl":
			ev.Level = normalizeLevel(v)
		case "msg", "message", "body":
			ev.Body = v
		case "time", "timestamp", "ts":
			ev.Timestamp = parseTime(v)
		case "trace_id", "traceid", "trace":
			ev.TraceID = v
		case "service":
			ev.Service = v
		default:
			ev.Attrs[k] = v
		}
	}
	if ev.Level == "" {
		ev.Level = model.LevelInfo
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	if ev.Body == "" {
		ev.Body = line
	}
	return ev, true
}

// parseLogfmtPairs is a minimal logfmt parser: key=value pairs separated by
// whitespace, values may be quoted with double quotes (with \" escapes).
func parseLogfmtPairs(line string) (map[string]string, bool) {
	out := map[string]string{}
	i := 0
	for i < len(line) {
		for i < len(line) && line[i] == ' ' {
			i++
		}
		if i >= len(line) {
			break
		}
		// key
		start := i
		for i < len(line) && line[i] != '=' && line[i] != ' ' {
			i++
		}
		if i >= len(line) || line[i] != '=' {
			return nil, false
		}
		key := line[start:i]
		i++ // skip '='
		// value
		var val string
		if i < len(line) && line[i] == '"' {
			i++
			vstart := i
			var sb strings.Builder
			for i < len(line) {
				if line[i] == '\\' && i+1 < len(line) {
					sb.WriteByte(line[i+1])
					i += 2
					continue
				}
				if line[i] == '"' {
					break
				}
				sb.WriteByte(line[i])
				i++
			}
			_ = vstart
			val = sb.String()
			if i < len(line) && line[i] == '"' {
				i++
			}
		} else {
			vstart := i
			for i < len(line) && line[i] != ' ' {
				i++
			}
			val = line[vstart:i]
		}
		out[key] = val
	}
	return out, true
}

func normalizeLevel(s string) model.LogLevel {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TRACE", "DEBUG":
		return model.LevelDebug
	case "INFO", "NOTICE":
		return model.LevelInfo
	case "WARN", "WARNING":
		return model.LevelWarn
	case "ERROR", "ERR", "FATAL", "CRITICAL":
		return model.LevelError
	default:
		return model.LevelInfo
	}
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

var timeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000Z",
	"2006-01-02 15:04:05",
	time.RFC1123,
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		// Heuristic: > 1e12 ⇒ ms epoch; > 1e15 ⇒ µs; else seconds.
		switch {
		case n > 1e15:
			return time.Unix(0, int64(n*1e3))
		case n > 1e12:
			return time.UnixMilli(int64(n))
		default:
			sec, frac := int64(n), n-float64(int64(n))
			return time.Unix(sec, int64(frac*1e9))
		}
	}
	for _, f := range timeFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
