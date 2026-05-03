package receiver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

// OSSidecarReceiver polls process-level CPU/memory at a fixed interval. It
// supports auto-PID discovery by process name and works on linux (/proc) or
// darwin (`ps` subprocess).
type OSSidecarReceiver struct {
	name    string
	process string
	pid     int
	interval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	out    chan<- any

	// linux-only: previous CPU sample for delta calculation
	lastTotal uint64
	lastProc  uint64
	lastTime  time.Time
}

func NewOSSidecar(name, process string, pid int, interval time.Duration) *OSSidecarReceiver {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &OSSidecarReceiver{
		name:     name,
		process:  process,
		pid:      pid,
		interval: interval,
	}
}

func (r *OSSidecarReceiver) Name() string { return r.name }

func (r *OSSidecarReceiver) Start(ctx context.Context, out chan<- any) error {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return errors.New("os sidecar: already started")
	}
	subCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.out = out
	r.mu.Unlock()
	go r.run(subCtx)
	return nil
}

func (r *OSSidecarReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	return nil
}

func (r *OSSidecarReceiver) emit(ev any) {
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

func (r *OSSidecarReceiver) run(ctx context.Context) {
	if r.pid == 0 && r.process != "" {
		if pid, err := findPID(r.process); err == nil {
			r.pid = pid
		}
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.pid == 0 && r.process != "" {
				if pid, err := findPID(r.process); err == nil {
					r.pid = pid
				} else {
					continue
				}
			}
			if r.pid <= 0 {
				continue
			}
			r.sample()
		}
	}
}

func (r *OSSidecarReceiver) sample() {
	switch runtime.GOOS {
	case "linux":
		r.sampleLinux()
	case "darwin":
		r.sampleDarwin()
	}
}

func (r *OSSidecarReceiver) sampleDarwin() {
	out, err := exec.Command("ps", "-o", "pcpu=,rss=", "-p", strconv.Itoa(r.pid)).Output()
	if err != nil {
		return
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return
	}
	pcpu, _ := strconv.ParseFloat(fields[0], 64)
	rssKB, _ := strconv.ParseFloat(fields[1], 64)
	now := time.Now()
	r.emit(model.MetricEvent{
		Name: "process.cpu.percent", Value: pcpu, Kind: model.Gauge,
		Timestamp: now, Source: "os",
	})
	r.emit(model.MetricEvent{
		Name: "process.memory.rss", Value: rssKB * 1024, Kind: model.Gauge,
		Timestamp: now, Source: "os",
	})
}

func (r *OSSidecarReceiver) sampleLinux() {
	procStat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", r.pid))
	if err != nil {
		return
	}
	fields := splitProcStat(string(procStat))
	// Field offsets per `man proc` (1-indexed): utime=14, stime=15.
	if len(fields) < 24 {
		return
	}
	utime, _ := strconv.ParseUint(fields[13], 10, 64)
	stime, _ := strconv.ParseUint(fields[14], 10, 64)
	procTotal := utime + stime

	hostTotal, ok := readHostJiffies()
	now := time.Now()

	if r.lastTime.IsZero() || !ok {
		r.lastTotal, r.lastProc, r.lastTime = hostTotal, procTotal, now
	} else {
		dHost := float64(hostTotal - r.lastTotal)
		dProc := float64(procTotal - r.lastProc)
		if dHost > 0 {
			ncpu := float64(runtime.NumCPU())
			cpuPct := dProc / dHost * 100 * ncpu
			r.emit(model.MetricEvent{
				Name: "process.cpu.percent", Value: cpuPct, Kind: model.Gauge,
				Timestamp: now, Source: "os",
			})
		}
		r.lastTotal, r.lastProc, r.lastTime = hostTotal, procTotal, now
	}

	if rss, ok := readRSS(r.pid); ok {
		r.emit(model.MetricEvent{
			Name: "process.memory.rss", Value: float64(rss), Kind: model.Gauge,
			Timestamp: now, Source: "os",
		})
	}
}

// splitProcStat splits /proc/<pid>/stat into its fields, treating the comm
// (field 2) as a single field even when it contains spaces.
func splitProcStat(s string) []string {
	open := strings.IndexByte(s, '(')
	close := strings.LastIndexByte(s, ')')
	if open < 0 || close < 0 || close < open {
		return strings.Fields(s)
	}
	pre := strings.Fields(s[:open])
	comm := s[open+1 : close]
	post := strings.Fields(s[close+1:])
	out := make([]string, 0, len(pre)+1+len(post))
	out = append(out, pre...)
	out = append(out, comm)
	out = append(out, post...)
	return out
}

func readHostJiffies() (uint64, bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, false
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 8 || fields[0] != "cpu" {
		return 0, false
	}
	var total uint64
	for _, f := range fields[1:] {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return 0, false
		}
		total += v
	}
	return total, true
}

func readRSS(pid int) (uint64, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseUint(fields[1], 10, 64)
				if err == nil {
					return kb * 1024, true
				}
			}
		}
	}
	return 0, false
}

func findPID(name string) (int, error) {
	out, err := exec.Command("pgrep", "-f", name).Output()
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		if pid != os.Getpid() {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("pid not found for %q", name)
}
