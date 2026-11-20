package activity

import "testing"

func TestLog(t *testing.T) {
	changes := 0
	l := New(func() { changes++ })
	for i := range Max + 5 {
		l.Add("test", Info, "entry %d", i)
	}
	e := l.Entries()
	if len(e) != Max || e[0].Text != "entry 5" || e[len(e)-1].Source != "test" {
		t.Fatalf("got %d entries, first %q", len(e), e[0].Text)
	}
	l.Clear()
	if len(l.Entries()) != 0 || changes != Max+6 {
		t.Fatalf("clear: %d entries, %d changes", len(l.Entries()), changes)
	}
}
