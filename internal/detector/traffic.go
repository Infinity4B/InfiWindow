package detector

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// Sample is the traffic of the watched processes over the last interval.
type Sample struct {
	At         time.Time `json:"at"`
	UploadKBps float64   `json:"uploadKBps"`
	DownKBps   float64   `json:"downKBps"`
	Processes  int       `json:"processes"` // watched processes seen
	Conns      int       `json:"conns"`     // connected peer sockets, -1 unknown
}

// counters are cumulative byte counts per pid.
type counters map[int][2]uint64

// parseLine reads one nettop CSV line such as "UURemoteServer.3268,381370,297154,".
// header reports the ",bytes_in,bytes_out," line that starts each sample.
func parseLine(line string) (name string, pid int, in, out uint64, header, ok bool) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, ",") {
		return "", 0, 0, 0, true, false
	}
	parts := strings.Split(line, ",")
	if len(parts) < 3 {
		return "", 0, 0, 0, false, false
	}
	dot := strings.LastIndexByte(parts[0], '.')
	if dot <= 0 {
		return "", 0, 0, 0, false, false
	}
	pid, err := strconv.Atoi(parts[0][dot+1:])
	if err != nil {
		return "", 0, 0, 0, false, false
	}
	in, err1 := strconv.ParseUint(parts[1], 10, 64)
	out, err2 := strconv.ParseUint(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		return "", 0, 0, 0, false, false
	}
	return parts[0][:dot], pid, in, out, false, true
}

func matches(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// rates turns two consecutive snapshots into KB/s. Processes absent from
// prev (new ones) contribute nothing until their second snapshot.
func rates(prev, cur counters, dt time.Duration) (upKBps, downKBps float64) {
	if dt <= 0 {
		return 0, 0
	}
	var up, down uint64
	for pid, c := range cur {
		p, ok := prev[pid]
		if !ok {
			continue
		}
		if c[0] >= p[0] {
			down += c[0] - p[0]
		}
		if c[1] >= p[1] {
			up += c[1] - p[1]
		}
	}
	s := dt.Seconds() * 1024
	return float64(up) / s, float64(down) / s
}

// Watcher streams traffic samples of processes whose name starts with one of
// the prefixes, using macOS's nettop (no root needed).
type Watcher struct {
	mu       sync.Mutex
	prefixes []string
}

// NewWatcher watches processes named with any of the prefixes.
func NewWatcher(prefixes []string) *Watcher {
	return &Watcher{prefixes: prefixes}
}

// SetPrefixes changes the watched process names.
func (w *Watcher) SetPrefixes(prefixes []string) {
	w.mu.Lock()
	w.prefixes = prefixes
	w.mu.Unlock()
}

func (w *Watcher) getPrefixes() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.prefixes...)
}

// Run sends a sample about every second until ctx is done. errs receives
// problems starting nettop; Run keeps retrying.
func (w *Watcher) Run(ctx context.Context, samples chan<- Sample, errs chan<- error) {
	conns := newConnCounter()
	go conns.run(ctx, w.getPrefixes)
	backoff := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := w.runNettop(ctx, samples, conns)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			select {
			case errs <- err:
			default:
			}
		}
		if time.Since(started) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (w *Watcher) runNettop(ctx context.Context, samples chan<- Sample, conns *connCounter) error {
	// -P: per process, -L 0: log forever, -s 1: every second, -x: raw
	// numbers. nettop buffers its output unless it writes to a terminal,
	// hence the pty.
	cmd := exec.CommandContext(ctx, "/usr/bin/nettop", "-P", "-L", "0", "-s", "1", "-x", "-J", "bytes_in,bytes_out")
	f, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = f.Close()
		_ = cmd.Wait()
	}()
	return readSamples(ctx, f, w.getPrefixes, conns.get, samples, time.Now)
}

func readSamples(ctx context.Context, r io.Reader, prefixes func() []string, conns func() int,
	samples chan<- Sample, now func() time.Time) error {
	sc := bufio.NewScanner(r)
	var (
		prev, cur counters
		prevAt    time.Time
		pfx       = prefixes()
	)
	flush := func() {
		if cur == nil {
			return
		}
		at := now()
		if prev != nil {
			up, down := rates(prev, cur, at.Sub(prevAt))
			s := Sample{At: at, UploadKBps: up, DownKBps: down, Processes: len(cur), Conns: conns()}
			select {
			case samples <- s:
			case <-ctx.Done():
			}
		}
		prev, prevAt = cur, at
		cur = nil
	}
	for sc.Scan() {
		name, pid, in, out, header, ok := parseLine(sc.Text())
		if header {
			flush()
			cur = counters{}
			pfx = prefixes()
			continue
		}
		if ok && cur != nil && matches(name, pfx) {
			cur[pid] = [2]uint64{in, out}
		}
	}
	// A block cut short by EOF is incomplete: drop it.
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

// connCounter counts connected sockets (TCP established or UDP with a peer)
// of the watched processes with lsof, every few seconds.
type connCounter struct {
	mu sync.Mutex
	n  int
}

func newConnCounter() *connCounter { return &connCounter{n: -1} }

func (c *connCounter) get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func (c *connCounter) run(ctx context.Context, prefixes func() []string) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		n := countConns(ctx, prefixes())
		c.mu.Lock()
		c.n = n
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func countConns(ctx context.Context, prefixes []string) int {
	args := []string{"-nP", "-i", "-a"}
	for _, p := range prefixes {
		if p != "" {
			args = append(args, "-c", p)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/sbin/lsof", args...).Output()
	if err != nil && len(out) == 0 {
		// lsof exits 1 when nothing matched.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return 0
		}
		return -1
	}
	return countConnLines(string(out))
}

func countConnLines(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "->") && !strings.Contains(line, "(LISTEN)") &&
			!strings.Contains(line, "->127.0.0.1") && !strings.Contains(line, "->[::1]") {
			n++
		}
	}
	return n
}
