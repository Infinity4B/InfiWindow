//go:build manual

package display

import "testing"

func TestManualList(t *testing.T) {
	for _, d := range ListDisplays() {
		t.Logf("%+v", d)
	}
}
