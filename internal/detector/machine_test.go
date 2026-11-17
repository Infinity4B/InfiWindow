package detector

import (
	"testing"
	"time"
)

func feed(m *Machine, start time.Time, rates []float64, conns int) (changes []int) {
	for i, r := range rates {
		if m.Step(start.Add(time.Duration(i)*time.Second), r, conns) {
			changes = append(changes, i)
		}
	}
	return changes
}

func TestConnectNeedsSustainedUpload(t *testing.T) {
	m := NewMachine(Params{ConnectKBps: 100, ConnectSeconds: 3, HoldKBps: 5, DisconnectSeconds: 10})
	t0 := time.Unix(0, 0)
	// A single burst (e.g. the device list refreshing) is ignored.
	if ch := feed(m, t0, []float64{0, 500, 0, 500, 500, 0}, -1); len(ch) != 0 || m.Connected() {
		t.Fatalf("burst connected: %v", ch)
	}
	ch := feed(m, t0.Add(10*time.Second), []float64{150, 150, 150}, -1)
	if len(ch) != 1 || ch[0] != 2 || !m.Connected() {
		t.Fatalf("expected connect at the third sample, got %v", ch)
	}
}

func TestDisconnectAfterQuietPeriod(t *testing.T) {
	m := NewMachine(Params{ConnectKBps: 100, ConnectSeconds: 1, HoldKBps: 5, DisconnectSeconds: 5})
	t0 := time.Unix(0, 0)
	feed(m, t0, []float64{300}, -1)
	if !m.Connected() {
		t.Fatal("should be connected")
	}
	// Static screen: low but non-zero upload keeps the session.
	if ch := feed(m, t0.Add(time.Second), []float64{8, 7, 9, 6, 8, 7, 8, 9}, -1); len(ch) != 0 {
		t.Fatalf("idle stream dropped: %v", ch)
	}
	// The last active sample was at t0+8s, so the session ends at t0+13s.
	ch := feed(m, t0.Add(9*time.Second), []float64{1, 1, 1, 1, 1, 1, 1}, -1)
	if len(ch) != 1 || ch[0] != 4 || m.Connected() {
		t.Fatalf("expected disconnect after 5s, got %v", ch)
	}
}

func TestExtraConnectionsHoldSession(t *testing.T) {
	m := NewMachine(Params{ConnectKBps: 100, ConnectSeconds: 1, HoldKBps: 5, DisconnectSeconds: 3})
	t0 := time.Unix(0, 0)
	feed(m, t0, []float64{0, 0}, 2) // idle baseline: 2 connections
	feed(m, t0.Add(2*time.Second), []float64{400}, 4)
	if ch := feed(m, t0.Add(3*time.Second), []float64{0, 0, 0, 0, 0, 0}, 4); len(ch) != 0 {
		t.Fatalf("open peer connection should hold the session: %v", ch)
	}
	if ch := feed(m, t0.Add(9*time.Second), []float64{0, 0, 0, 0}, 2); len(ch) != 1 {
		t.Fatalf("expected disconnect once peers closed: %v", ch)
	}
}

func TestNormalize(t *testing.T) {
	p := Params{ConnectKBps: 10, HoldKBps: 50, DisconnectSeconds: 1}.Normalize()
	if p.HoldKBps != 10 || p.ConnectSeconds < 1 || p.DisconnectSeconds < 3 {
		t.Fatalf("bad normalize: %+v", p)
	}
}
