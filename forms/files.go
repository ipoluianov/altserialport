package forms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ipoluianov/altserialport/app"
	"github.com/ipoluianov/altserialport/buffer"
	"github.com/ipoluianov/altserialport/config"
	"github.com/ipoluianov/altserialport/proto"
	"github.com/ipoluianov/altserialport/proto/xmodem"
	"github.com/ipoluianov/altserialport/render"
	"github.com/ipoluianov/altserialport/session"
	"github.com/ipoluianov/nui/ui"
)

// exportHTML writes the rows as the terminal shows them, with their colors
func (m *MainForm) exportHTML() []byte {
	var sb strings.Builder
	css := func(c color.RGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
	fmt.Fprintf(&sb, "<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>%s</title>\n", app.DisplayName)
	fmt.Fprintf(&sb, "<style>body{background:%s;color:%s;font:13px/1.45 'JetBrains Mono',Consolas,monospace;white-space:pre-wrap}"+
		".t{color:%s}.n{color:%s;font-style:italic}.g{color:%s}</style></head><body>\n",
		css(colorTermBack.get()), css(colorTermText.get()), css(colorSent.get()), css(colorNote.get()), css(colorGutter.get()))
	t := m.term
	for i := m.buf.FirstRow(); i < m.buf.EndRow(); i++ {
		r := m.buf.Row(i)
		if g := t.gutter(i, r); g != "" {
			sb.WriteString(`<span class="g">` + html.EscapeString(g) + " </span>")
		}
		class := ""
		switch r.Dir {
		case buffer.TX:
			class = "t"
		case buffer.Info:
			class = "n"
		}
		rowColor := ""
		if c := t.rowColor(i); c != "" && class == "" {
			rowColor = c
		}
		sb.WriteString(`<span class="` + class + `">`)
		cells := t.cells(i)
		for k := 0; k < len(cells); {
			// A run of one color
			c := cells[k]
			j := k
			for j < len(cells) && cells[j].FG == c.FG && cells[j].BG == c.BG && cells[j].A == c.A {
				j++
			}
			text := render.String(cells[k:j])
			style := ""
			if c.FG != 0 {
				style += "color:" + css(cellColor(c.FG, colorTermText.get())) + ";"
			} else if rowColor != "" {
				style += "color:" + rowColor + ";"
			}
			if c.BG != 0 {
				style += "background:" + css(cellColor(c.BG, colorTermBack.get())) + ";"
			}
			if c.A&render.Bold != 0 {
				style += "font-weight:bold;"
			}
			if c.A&render.Token != 0 {
				style += "color:" + css(colorToken.get()) + ";"
			}
			if style != "" {
				sb.WriteString(`<span style="` + style + `">` + html.EscapeString(text) + "</span>")
			} else {
				sb.WriteString(html.EscapeString(text))
			}
			k = j
		}
		sb.WriteString("</span>\n")
	}
	sb.WriteString("</body></html>\n")
	return []byte(sb.String())
}

// exportHexDump writes every chunk kept: its time, its direction and its
// bytes in rows of sixteen, whatever the view
func (m *MainForm) exportHexDump() []byte {
	var sb strings.Builder
	first, end := m.buf.Chunks()
	names := map[buffer.Dir]string{buffer.RX: "RX", buffer.TX: "TX", buffer.Keys: "TX", buffer.Info: "--"}
	for ci := first; ci < end; ci++ {
		c := m.buf.Chunk(ci)
		if c == nil {
			continue
		}
		ts := time.Unix(0, c.Time).Format("2006-01-02 15:04:05.000000")
		if c.Dir == buffer.Info {
			fmt.Fprintf(&sb, "%s -- %s\n", ts, c.Data)
			continue
		}
		fmt.Fprintf(&sb, "%s %s %d bytes\n", ts, names[c.Dir], len(c.Data))
		for off := 0; off < len(c.Data); off += 16 {
			row := c.Data[off:min(off+16, len(c.Data))]
			fmt.Fprintf(&sb, "  %04X  %s\n", off, render.String(render.Hex(nil, render.Input{Data: row}, 16)))
		}
	}
	return []byte(sb.String())
}

// openFile shows a saved log or any file in the terminal, without a port
func (m *MainForm) openFile() {
	if state, _, _ := m.sess.State(); state != session.Closed {
		ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, T().OpenWhileConnected)
		return
	}
	m.Form.ShowOpenFileDialog(ui.OpenFileDialogOptions{Title: T().OpenFile, DefaultDirectory: config.Get().LogDir},
		func(paths []string, err error) {
			if err == nil && len(paths) > 0 {
				m.loadFile(paths[0])
			}
		})
}

// loadFile puts the bytes of the file into the terminal as received data
func (m *MainForm) loadFile(path string) {
	if state, _, _ := m.sess.State(); state != session.Closed {
		ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, T().OpenWhileConnected)
		return
	}
	st, err := os.Stat(path)
	if err == nil && st.Size() > 256<<20 {
		err = fmt.Errorf(T().FileTooBig, formatBytes(st.Size()))
	}
	var data []byte
	if err == nil {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, err.Error())
		return
	}
	m.term.Clear()
	at := st.ModTime().UnixNano()
	chunks := []buffer.Chunk{{Time: at, Dir: buffer.Info, Data: []byte(fmt.Sprintf(T().FileOpened, filepath.Base(path), formatBytes(int64(len(data)))))}}
	for off := 0; off < len(data); off += 64 * 1024 {
		chunks = append(chunks, buffer.Chunk{Time: at, Dir: buffer.RX, Data: data[off:min(off+64*1024, len(data))]})
	}
	m.term.Append(chunks)
	m.term.ScrollToStart()
}

// startAutoLog records to a file in the folder of the logs when the
// profile of the device asks for it
func (m *MainForm) startAutoLog() {
	if !m.profile.AutoLog || m.sess.Log() != nil {
		return
	}
	dir := config.Get().LogDir
	if dir == "" {
		dir = filepath.Join(config.Dir(), "logs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.status.ShowMessage(err.Error())
		return
	}
	_, info, _ := m.sess.State()
	path := filepath.Join(dir, logName(info.Short, ".log"))
	if err := m.sess.StartLog(path, session.LogText, true); err != nil {
		m.status.ShowMessage(err.Error())
		return
	}
	m.limitLog()
	m.actRecord.SetChecked(true)
	m.recordHighlight(true)
}

// limitLog sets the size at which the log goes on in a new file
func (m *MainForm) limitLog() {
	if l := m.sess.Log(); l != nil {
		l.SetMaxSize(int64(config.Get().LogMaxMB) << 20)
	}
}

// logName is the name of a log: the port and the time
func logName(port, ext string) string {
	if port == "" {
		port = "serial"
	}
	port = strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(port)
	return fmt.Sprintf("%s_%s%s", port, time.Now().Format("2006-01-02_15-04-05"), ext)
}

// Named profiles

// saveNamedProfile keeps the settings of the device under a name
func (m *MainForm) saveNamedProfile() {
	m.saveProfile()
	ui.ShowInputDialog(m.Form.Panel(), ui.InputDialog{Title: T().Profiles, Prompt: T().ProfileName,
		Value: fmt.Sprintf("%d %s", m.cfg.Baud, m.cfg.Frame())}, func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		p := m.profile
		p.History = nil
		config.Update(func(s *config.Settings) {
			if s.Named == nil {
				s.Named = map[string]config.Profile{}
			}
			s.Named[name] = p
		})
		m.status.ShowMessage(fmt.Sprintf(T().ProfileSaved, name))
	})
}

// useProfile applies a profile to the device, keeping its command history
func (m *MainForm) useProfile(p config.Profile) {
	p.History = m.send.History()
	m.applyProfile(config.FixProfile(p))
	if state, _, _ := m.sess.State(); state != session.Closed {
		m.sess.SetConfig(m.cfg)
		m.sess.SetDTR(m.profile.DTR)
		m.sess.SetRTS(m.profile.RTS)
	}
	m.saveProfile()
	m.updateState()
}

// profilesMenu fills the menu of the named profiles when it opens
func (m *MainForm) profilesMenu(owner ui.Widgeter) *ui.ContextMenu {
	menu := ui.NewContextMenu(owner)
	menu.OnShow.Connect(func(menu *ui.ContextMenu) {
		menu.RemoveAllItems()
		menu.AddItem(T().ProfileSaveAs, m.saveNamedProfile)
		named := config.Get().Named
		var names []string
		for n := range named {
			names = append(names, n)
		}
		sort.Strings(names)
		if len(names) > 0 {
			menu.AddSeparator()
			for _, n := range names {
				p := named[n]
				menu.AddItem(n, func() { m.useProfile(p) })
			}
			del := ui.NewContextMenu(owner)
			for _, n := range names {
				n := n
				del.AddItem(n, func() {
					config.Update(func(s *config.Settings) { delete(s.Named, n) })
				})
			}
			menu.AddSeparator()
			menu.AddItemWithSubmenu(T().ProfileDelete, del)
		}
		menu.AddSeparator()
		menu.AddItem(T().ProfileExport, m.exportProfile)
		menu.AddItem(T().ProfileImport, m.importProfile)
	})
	return menu
}

// exportedProfile is the file of a profile
type exportedProfile struct {
	App     string         `json:"app"`
	Name    string         `json:"name"`
	Profile config.Profile `json:"profile"`
}

func (m *MainForm) exportProfile() {
	m.saveProfile()
	m.Form.ShowSaveFileDialog(ui.SaveFileDialogOptions{Title: T().ProfileExport,
		DefaultFileName: fmt.Sprintf("%d-%s.altserial.json", m.cfg.Baud, m.cfg.Frame()),
		Filters:         []ui.FileDialogFilter{{DisplayName: T().ProfileFiles, Patterns: []string{"*.json"}}},
	}, func(path string, err error) {
		if err != nil || path == "" {
			return
		}
		p := m.profile
		p.History = nil
		name := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".json"), ".altserial")
		data, _ := json.MarshalIndent(exportedProfile{App: app.DisplayName, Name: name, Profile: p}, "", "  ")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, err.Error())
		}
	})
}

func (m *MainForm) importProfile() {
	m.Form.ShowOpenFileDialog(ui.OpenFileDialogOptions{Title: T().ProfileImport,
		Filters: []ui.FileDialogFilter{{DisplayName: T().ProfileFiles, Patterns: []string{"*.json"}}},
	}, func(paths []string, err error) {
		if err != nil || len(paths) == 0 {
			return
		}
		data, err := os.ReadFile(paths[0])
		var e exportedProfile
		if err == nil {
			err = json.Unmarshal(data, &e)
		}
		if err == nil && e.Profile.Baud <= 0 {
			err = errors.New(T().ProfileBadFile)
		}
		if err != nil {
			ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, err.Error())
			return
		}
		if e.Name == "" {
			e.Name = filepath.Base(paths[0])
		}
		config.Update(func(s *config.Settings) {
			if s.Named == nil {
				s.Named = map[string]config.Profile{}
			}
			s.Named[e.Name] = e.Profile
		})
		m.useProfile(e.Profile)
		m.status.ShowMessage(fmt.Sprintf(T().ProfileSaved, e.Name))
	})
}

// File transfer

// fileMode is how a file is sent
type fileMode int

const (
	fileRaw fileMode = iota
	fileLines
	fileXModem
	fileXModem1K
	fileYModem
)

// sendFileDialog asks how to send a file
type sendFileDialog struct {
	ui.DialogContent
	OnOK func(mode fileMode, lineDelay, charDelay time.Duration)
}

func newSendFileDialog(name string) *sendFileDialog {
	d := &sendFileDialog{}
	d.InitWidget()
	mode := ui.NewComboBox()
	for i, n := range T().FileModes {
		mode.AddItem(n, fileMode(i))
	}
	lineDelay := ui.NewTextBox()
	lineDelay.SetText("50")
	charDelay := ui.NewTextBox()
	charDelay.SetText("0")
	fields := ui.NewFormLayout()
	fields.AddRow(T().FileLabel, ui.NewLabel(name))
	fields.AddRow(T().FileMode, mode)
	fields.AddRow(T().FileLineDelay, lineDelay)
	fields.AddRow(T().FileCharDelay, charDelay)
	update := func() {
		f, _ := mode.SelectedItemData().(fileMode)
		lineDelay.SetEnabled(f == fileLines)
		charDelay.SetEnabled(f == fileLines)
	}
	mode.OnSelectionChanged.Connect(func(int) { update() })
	update()
	d.AddWidget(0, 0, fields)
	d.AddVSpacer(1, 0)
	buttons := ui.NewDialogButtonBox()
	buttons.AddOK(func() {
		ld, err1 := strconv.Atoi(strings.TrimSpace(lineDelay.Text()))
		cd, err2 := strconv.Atoi(strings.TrimSpace(charDelay.Text()))
		if err1 != nil || err2 != nil || ld < 0 || cd < 0 {
			ui.ShowMessageBox(d, T().SendFile, T().FileBadDelay)
			return
		}
		f, _ := mode.SelectedItemData().(fileMode)
		d.RunInParent(func() {
			d.OnOK(f, time.Duration(ld)*time.Millisecond, time.Duration(cd)*time.Millisecond)
		})
		d.Form().Close()
	})
	buttons.AddCancel(func() { d.Form().Close() })
	d.AddWidget(2, 0, buttons)
	d.OnDialogShow = func() { d.Form().SetTitle(T().SendFile) }
	return d
}

// sendFile sends a file: as it is, line by line, or by XMODEM / YMODEM
func (m *MainForm) sendFile() {
	if state, _, _ := m.sess.State(); state != session.Open {
		m.status.ShowMessage(T().ErrNotOpen + " — " + T().PressConnect)
		return
	}
	m.Form.ShowOpenFileDialog(ui.OpenFileDialogOptions{Title: T().SendFile}, func(paths []string, err error) {
		if err != nil || len(paths) == 0 {
			return
		}
		data, err := os.ReadFile(paths[0])
		if err != nil {
			ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, err.Error())
			return
		}
		name := filepath.Base(paths[0])
		d := newSendFileDialog(name)
		d.OnOK = func(mode fileMode, lineDelay, charDelay time.Duration) {
			m.transferFile(name, data, mode, lineDelay, charDelay)
		}
		m.Form.Panel().ShowDialog(d)
	})
}

func (m *MainForm) transferFile(name string, data []byte, mode fileMode, lineDelay, charDelay time.Duration) {
	_, _, cfg := m.sess.State()
	progress := func(p *ui.Progress, done, total int) {
		if total > 0 {
			p.Report(float64(done)/float64(total), fmt.Sprintf("%s / %s", formatBytes(int64(done)), formatBytes(int64(total))))
		}
	}
	// The protocol takes the port before the work starts: the first request
	// of a receiver that is already waiting is not lost
	var link *xmodem.ChanLink
	done := func() {}
	if mode >= fileXModem {
		link, done = m.protocolLink()
	}
	ui.RunWithProgress(m.Form.Panel(), T().SendFile, name, func(ctx context.Context, p *ui.Progress) (int, error) {
		defer done()
		switch mode {
		case fileXModem, fileXModem1K, fileYModem:
			proto := map[fileMode]xmodem.Protocol{fileXModem: xmodem.XModem, fileXModem1K: xmodem.XModem1K, fileYModem: xmodem.YModem}[mode]
			p.Report(0, T().FileWaitingReceiver)
			sent := 0
			err := xmodem.Send(ctx, link, proto, name, data, func(d, total int) {
				sent = d
				progress(p, d, total)
			})
			return sent, err
		case fileLines:
			lines := bytes.SplitAfter(data, []byte{'\n'})
			ending := proto.LineEndingByName(m.send.Ending()).Bytes()
			sent := 0
			for i, line := range lines {
				if ctx.Err() != nil {
					return sent, ctx.Err()
				}
				line = bytes.TrimRight(line, "\r\n")
				out := append(append([]byte(nil), line...), ending...)
				if charDelay > 0 {
					for _, c := range out {
						if err := m.sess.WriteSync([]byte{c}); err != nil {
							return sent, err
						}
						time.Sleep(charDelay)
					}
				} else if err := m.sess.WriteSync(out); err != nil {
					return sent, err
				}
				sent += len(line)
				progress(p, i+1, len(lines))
				time.Sleep(lineDelay + time.Duration(len(out))*cfg.ByteTime())
			}
			return sent, nil
		}
		const piece = 256
		sent := 0
		for sent < len(data) {
			if ctx.Err() != nil {
				return sent, ctx.Err()
			}
			end := min(sent+piece, len(data))
			if err := m.sess.WriteSync(data[sent:end]); err != nil {
				return sent, err
			}
			// The pieces go no faster than the line: the progress is true
			time.Sleep(time.Duration(end-sent) * cfg.ByteTime())
			sent = end
			progress(p, sent, len(data))
		}
		return sent, nil
	}, func(sent int, err error) {
		switch {
		case err != nil && ctxErr(err):
			m.status.ShowMessage(fmt.Sprintf(T().FileCancelled, formatBytes(int64(sent))))
		case err != nil:
			ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, m.errorText(err))
		default:
			m.status.ShowMessage(fmt.Sprintf(T().FileSent, name, formatBytes(int64(len(data)))))
		}
	})
}

// protocolLink gives a file protocol the port: the received bytes go to it
// and out of the terminal until done is called
func (m *MainForm) protocolLink() (*xmodem.ChanLink, func()) {
	link := xmodem.NewChanLink(func(b []byte) error { return m.sess.WriteSync(b) })
	remove := m.sess.AddTap(func(c buffer.Chunk) {
		if c.Dir == buffer.RX {
			link.Feed(c.Data)
		}
	})
	m.sess.SetHidden(true)
	return link, func() {
		remove()
		m.sess.SetHidden(false)
	}
}

// receiveFile takes a file the device sends by XMODEM or YMODEM
func (m *MainForm) receiveFile(p xmodem.Protocol) {
	if state, _, _ := m.sess.State(); state != session.Open {
		m.status.ShowMessage(T().ErrNotOpen + " — " + T().PressConnect)
		return
	}
	type got struct {
		name string
		data []byte
	}
	ui.RunWithProgress(m.Form.Panel(), T().ReceiveFile, T().FileWaitingSender, func(ctx context.Context, pr *ui.Progress) (got, error) {
		link, done := m.protocolLink()
		defer done()
		name, data, err := xmodem.Receive(ctx, link, p, func(d, total int) {
			if total > 0 {
				pr.Report(float64(d)/float64(total), formatBytes(int64(d)))
			} else {
				pr.Report(0, formatBytes(int64(d)))
			}
		})
		return got{name, data}, err
	}, func(g got, err error) {
		if err != nil {
			if !ctxErr(err) {
				ui.ShowMessageBox(m.Form.Panel(), T().ReceiveFile, err.Error())
			}
			return
		}
		if g.name == "" {
			g.name = "received.bin"
		}
		m.Form.ShowSaveFileDialog(ui.SaveFileDialogOptions{Title: T().ReceiveFile, DefaultFileName: g.name},
			func(path string, err error) {
				if err != nil || path == "" {
					return
				}
				if err := os.WriteFile(path, g.data, 0o644); err != nil {
					ui.ShowMessageBox(m.Form.Panel(), app.DisplayName, err.Error())
					return
				}
				m.status.ShowMessage(fmt.Sprintf(T().LogSaved, path))
			})
	})
}
