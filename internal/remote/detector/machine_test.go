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
	// The 5-second average drops below 5 KB/s at t0+11s (8+9+1+1+1).
	ch := feed(m, t0.Add(9*time.Second), []float64{1, 1, 1, 1, 1, 1, 1}, -1)
	if len(ch) != 1 || ch[0] != 2 || m.Connected() {
		t.Fatalf("expected disconnect at t0+11s, got %v", ch)
	}
}

func TestIdleBurstsDoNotHoldSession(t *testing.T) {
	m := NewMachine(Params{ConnectKBps: 120, ConnectSeconds: 1, HoldKBps: 6, DisconnectSeconds: 25})
	t0 := time.Unix(0, 0)
	feed(m, t0, []float64{300}, 2)
	// After the remote side leaves, UU Remote keeps sending a short burst
	// every ~15s (measured: 10-19 KB/s for a second, ~0 otherwise).
	idle := make([]float64, 60)
	for i := range idle {
		if i%15 == 7 {
			idle[i] = 19
		} else if i%15 == 8 {
			idle[i] = 4
		}
	}
	ch := feed(m, t0.Add(time.Second), idle, 2)
	if len(ch) != 1 || m.Connected() {
		t.Fatalf("bursts held the session: %v", ch)
	}
	if ch[0] != 24 { // a full 25s window after the connect at t0
		t.Fatalf("expected disconnect at t0+25s, got sample %d", ch[0])
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
