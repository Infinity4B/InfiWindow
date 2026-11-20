package display

import "time"

// ModeSettle is how long a display must keep a new mode, with no remote
// session running, before it counts as the mode the user chose. Remote
// desktop apps switch the resolution the moment they connect (UU Remote
// puts the built-in screen at 1800×1169 @59.94 Hz), and the session is
// detected well within this time, so such a mode is never learned.
const ModeSettle = 10 * time.Second

// baseline learns each display's chosen mode from observations made while
// idle. The first mode seen for a display is taken at once, since nothing
// better is known yet; later changes have to hold for settle.
type baseline[M comparable] struct {
	settle  time.Duration
	chosen  map[uint32]M
	pending map[uint32]candidate[M]
}

type candidate[M comparable] struct {
	mode  M
	since time.Time
}

func newBaseline[M comparable](settle time.Duration) *baseline[M] {
	return &baseline[M]{settle: settle, chosen: map[uint32]M{}, pending: map[uint32]candidate[M]{}}
}

// observe records the current modes of the online displays at now.
func (b *baseline[M]) observe(now time.Time, cur map[uint32]M) {
	for id, md := range cur {
		chosen, known := b.chosen[id]
		if !known {
			b.chosen[id] = md
			continue
		}
		if md == chosen {
			delete(b.pending, id)
			continue
		}
		p, ok := b.pending[id]
		if !ok || p.mode != md {
			b.pending[id] = candidate[M]{mode: md, since: now}
			continue
		}
		if now.Sub(p.since) >= b.settle {
			b.chosen[id] = md
			delete(b.pending, id)
		}
	}
	for id := range b.pending {
		if _, ok := cur[id]; !ok {
			delete(b.pending, id)
		}
	}
}

// interrupt forgets pending changes: a mode seen before a session must be
// seen for a whole settle period again afterwards.
func (b *baseline[M]) interrupt() { clear(b.pending) }

// adopt takes cur as the chosen modes right away.
func (b *baseline[M]) adopt(cur map[uint32]M) {
	for id, md := range cur {
		b.chosen[id] = md
	}
	b.interrupt()
}

// get returns the chosen mode of display id.
func (b *baseline[M]) get(id uint32) (M, bool) {
	md, ok := b.chosen[id]
	return md, ok
}
