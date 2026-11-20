//go:build manual

// CGO_ENABLED=0 go test -tags manual -run Live -v ./internal/remote/detector
package detector

import (
	"context"
	"testing"
	"time"
)

func TestLiveWatcher(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	samples := make(chan Sample)
	errs := make(chan error, 1)
	go NewWatcher([]string{"UURemote"}).Run(ctx, samples, errs)
	n := 0
	for {
		select {
		case s := <-samples:
			n++
			t.Logf("up %.1f KB/s down %.1f KB/s procs %d conns %d", s.UploadKBps, s.DownKBps, s.Processes, s.Conns)
		case err := <-errs:
			t.Fatal(err)
		case <-ctx.Done():
			if n < 3 {
				t.Fatalf("only %d samples", n)
			}
			return
		}
	}
}
