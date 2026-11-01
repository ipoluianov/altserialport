package forms

import (
	"testing"

	"github.com/ipoluianov/nui/ui"
	"github.com/ipoluianov/nui/ui/uitest"
)

// In a wide window the bars take its whole width: the menu button is at
// the right edge
func TestWideWindow(t *testing.T) {
	m, w, _ := newTestForm(t)
	uitest.SetScreen(2000, 1000, 1)
	w.Resize(1900, 980)
	w.Settle()
	shot(t, w, "15-wide")
	for _, c := range m.Form.Panel().Widgets() {
		if c.IsVisible() && c.Width() < 1850 {
			t.Errorf("%T is %d wide in a window of 1900", c, c.Width())
		}
	}
	// The menu button is the first one, the menu opens under it
	x, y := m.menuBtn.RectClientAreaOnWindow()
	if x > 10 {
		t.Errorf("the menu button is at %d", x)
	}
	w.Click(m.menuBtn)
	w.Settle()
	pop := w.Popup()
	if pop == nil {
		t.Fatal("no menu")
	}
	if pop.X() < x-4 || pop.X() > x+4 || pop.Y() < y+m.menuBtn.Height()-4 {
		t.Errorf("the menu at %d,%d, the button at %d,%d", pop.X(), pop.Y(), x, y)
	}
	shot(t, w, "16-menu")
	_ = ui.KeyF5
}
