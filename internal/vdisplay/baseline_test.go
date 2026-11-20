package vdisplay

import (
	"testing"
	"time"
)

func TestBaselineTakesFirstModeAtOnce(t *testing.T) {
	b := newBaseline[int](10 * time.Second)
	b.observe(time.Unix(0, 0), map[uint32]int{1: 54, 5: 60})
	if md, ok := b.get(1); !ok || md != 54 {
		t.Fatalf("display 1 = %v %v, want 54", md, ok)
	}
	if md, ok := b.get(5); !ok || md != 60 {
		t.Fatalf("display 5 = %v %v, want 60", md, ok)
	}
	if _, ok := b.get(7); ok {
		t.Fatal("unknown display has a mode")
	}
}

func TestBaselineAdoptsSettledChange(t *testing.T) {
	b := newBaseline[int](10 * time.Second)
	t0 := time.Unix(0, 0)
	b.observe(t0, map[uint32]int{1: 54})
	b.observe(t0.Add(time.Second), map[uint32]int{1: 66})
	b.observe(t0.Add(10*time.Second), map[uint32]int{1: 66})
	if md, _ := b.get(1); md != 54 {
		t.Fatalf("adopted after 9s: %v", md)
	}
	b.observe(t0.Add(11*time.Second), map[uint32]int{1: 66})
	if md, _ := b.get(1); md != 66 {
		t.Fatalf("not adopted after 10s: %v", md)
	}
}

// A remote app switches the mode as it connects; the session is detected
// a few seconds later and interrupts learning, so the mode is not taken.
func TestBaselineIgnoresModeChangedAtSessionStart(t *testing.T) {
	b := newBaseline[int](10 * time.Second)
	t0 := time.Unix(0, 0)
	b.observe(t0, map[uint32]int{1: 54})
	b.observe(t0.Add(5*time.Second), map[uint32]int{1: 68}) // UU connects
	b.interrupt()                                           // session detected
	// Session over; WindowServer still shows UU's mode until restored.
	b.observe(t0.Add(60*time.Second), map[uint32]int{1: 68})
	b.observe(t0.Add(65*time.Second), map[uint32]int{1: 68})
	if md, _ := b.get(1); md != 54 {
		t.Fatalf("baseline = %v, want 54", md)
	}
}

func TestBaselineChangeBackCancelsPending(t *testing.T) {
	b := newBaseline[int](10 * time.Second)
	t0 := time.Unix(0, 0)
	b.observe(t0, map[uint32]int{1: 54})
	b.observe(t0.Add(time.Second), map[uint32]int{1: 66})
	b.observe(t0.Add(2*time.Second), map[uint32]int{1: 54})
	b.observe(t0.Add(3*time.Second), map[uint32]int{1: 66})
	b.observe(t0.Add(12*time.Second), map[uint32]int{1: 66})
	if md, _ := b.get(1); md != 54 {
		t.Fatalf("pending period did not restart: %v", md)
	}
}

func TestBaselineKeepsOfflineDisplays(t *testing.T) {
	b := newBaseline[int](10 * time.Second)
	t0 := time.Unix(0, 0)
	b.observe(t0, map[uint32]int{1: 54, 5: 60})
	b.observe(t0.Add(time.Second), map[uint32]int{1: 54, 5: 61})
	b.observe(t0.Add(2*time.Second), map[uint32]int{1: 54}) // unplugged
	b.observe(t0.Add(30*time.Second), map[uint32]int{1: 54, 5: 61})
	if md, _ := b.get(5); md != 60 {
		t.Fatalf("display 5 = %v, want 60 (pending dropped while offline)", md)
	}
}

func TestBaselineAdopt(t *testing.T) {
	b := newBaseline[int](10 * time.Second)
	t0 := time.Unix(0, 0)
	b.observe(t0, map[uint32]int{1: 54, 5: 60})
	b.observe(t0.Add(time.Second), map[uint32]int{1: 66})
	b.adopt(map[uint32]int{1: 66})
	if md, _ := b.get(1); md != 66 {
		t.Fatalf("display 1 = %v, want 66", md)
	}
	if md, _ := b.get(5); md != 60 {
		t.Fatalf("offline display 5 = %v, want 60", md)
	}
}
