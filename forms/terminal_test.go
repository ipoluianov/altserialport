package forms

import (
	"fmt"
	"testing"
	"time"

	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/nui/ui"
	"github.com/ipoluianov/nui/ui/uitest"
)

// A million lines: adding them, a frame and a scroll stay quick
func TestTerminalMillionLines(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	buf := buffer.New(buffer.DefaultOptions(), 256<<20, 1_000_000)
	term := NewTerminal(buf)
	f := ui.NewForm()
	f.SetSize(1000, 600)
	f.Panel().AddWidget(0, 0, term)
	w := uitest.Show(t, f)

	start := time.Now()
	now := time.Now().UnixNano()
	var chunks []buffer.Chunk
	for i := 0; i < 1_000_000; i++ {
		line := fmt.Sprintf("I (%d) app: value=%d \x1b[0;32mok\x1b[0m\r\n", i, i*7)
		chunks = append(chunks, buffer.Chunk{Time: now + int64(i), Dir: buffer.RX, Data: []byte(line)})
		if len(chunks) == 1000 {
			term.Append(chunks)
			chunks = chunks[:0]
		}
	}
	appendTime := time.Since(start)

	start = time.Now()
	w.Settle()
	w.Screenshot()
	frame := time.Since(start)

	start = time.Now()
	term.vbar.SetValue(500_000)
	term.top = buf.FirstRow() + 500_000
	term.setFollow(false)
	term.area.Update()
	w.Settle()
	w.Screenshot()
	scroll := time.Since(start)

	start = time.Now()
	n, _ := term.SetQuery("value=6999993", false)
	find := time.Since(start)

	t.Logf("rows %d, append %v, frame %v, frame after a jump %v, find %v (%d)",
		buf.RowCount(), appendTime, frame, scroll, find, n)
	if n != 1 {
		t.Fatal("matches", n)
	}
	if frame > 500*time.Millisecond || scroll > 500*time.Millisecond {
		t.Fatal("slow frames")
	}
}
