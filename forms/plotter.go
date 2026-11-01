package forms

import (
	"time"

	"github.com/ipoluianov/altserialport/plot"
	"github.com/ipoluianov/nui/ui"
)

const (
	maxSeries      = 16
	maxPlotPoints  = 300_000 // a series keeps this many, then the oldest go
	plotKeepPoints = 200_000
)

// series is a line of the chart and the points it has
type series struct {
	src    *ui.TimeChartMemorySource
	points []ui.TimeChartPoint
}

// Plotter is the live chart of the numbers in the received lines
type Plotter struct {
	ui.Panel
	chart  *ui.TimeChart
	area   *ui.TimeChartArea
	series map[string]*series
	window *ui.ComboBox
	info   *ui.Label
	frozen bool
	dirty  bool

	OnClose func()
}

// NewPlotter makes the chart panel
func NewPlotter() *Plotter {
	p := &Plotter{series: map[string]*series{}}
	p.InitWidget()
	p.SetPanelPadding(0)
	p.SetCellPadding(0)

	title := ui.NewLabel("")
	title.SetTextFunc(func() string { return T().PlotTitle })
	title.SetFontBold(true)
	p.info = ui.NewLabel("")
	p.info.SetForegroundColor(colorSecondary.get())
	p.window = ui.NewComboBox()
	for _, s := range []int{10, 60, 600, 3600} {
		p.window.AddItem(formatWindow(s), s)
	}
	p.window.SetSelectedIndex(1)
	p.window.SetMaxWidth(110)
	p.window.SetTooltipFunc(func() string { return T().PlotWindowTip })
	p.window.OnActivated.Connect(func(int) {
		p.chart.ResetZoom()
		p.follow()
	})

	head := ui.NewHBox()
	head.Add(fixedSpace(8))
	head.Add(title)
	head.Add(fixedSpace(12))
	head.Add(p.info)
	head.AddStretch()
	head.Add(p.window)
	head.Add(iconButton("clear", func() string { return T().PlotClear }, p.Clear))
	head.Add(iconButton("close", func() string { return T().FindClose }, func() {
		if p.OnClose != nil {
			p.OnClose()
		}
	}))
	p.AddWidget(0, 0, head)

	p.chart = ui.NewTimeChart()
	p.chart.SetMinSize(200, 120)
	p.area = p.chart.AddArea()
	p.AddWidget(1, 0, p.chart)
	p.AddTimer(100, func() {
		if p.dirty && p.IsVisible() {
			p.dirty = false
			p.follow()
		}
	})
	p.showInfo()
	return p
}

func formatWindow(s int) string {
	names := T().PlotWindows
	switch s {
	case 10:
		return names[0]
	case 60:
		return names[1]
	case 600:
		return names[2]
	}
	return names[3]
}

// follow moves the chart to the newest points
func (p *Plotter) follow() {
	w, _ := p.window.SelectedItemData().(int)
	now := time.Now()
	p.chart.SetDefaultTimeRange(now.Add(-time.Duration(w)*time.Second), now)
	p.chart.Update()
}

// AddLine takes the numbers of a received line, at its time
func (p *Plotter) AddLine(line string, at time.Time) {
	vals := plot.Parse(line)
	if len(vals) == 0 {
		return
	}
	for _, v := range vals {
		s := p.series[v.Name]
		if s == nil {
			if len(p.series) >= maxSeries {
				continue
			}
			s = &series{src: ui.NewTimeChartMemorySource()}
			p.series[v.Name] = s
			p.area.AddSeries(v.Name, s.src)
			p.showInfo()
		}
		pt := ui.NewTimeChartValue(at, v.V)
		s.points = append(s.points, pt)
		if len(s.points) > maxPlotPoints {
			s.points = append([]ui.TimeChartPoint(nil), s.points[len(s.points)-plotKeepPoints:]...)
			s.src.SetPoints(s.points)
		} else {
			s.src.AddPoint(pt)
		}
	}
	p.dirty = true
}

// Clear drops all the series
func (p *Plotter) Clear() {
	p.chart.RemoveAllAreas()
	p.area = p.chart.AddArea()
	p.series = map[string]*series{}
	p.showInfo()
	p.chart.Update()
}

func (p *Plotter) showInfo() {
	if len(p.series) == 0 {
		p.info.SetText(T().PlotHint)
	} else {
		p.info.SetText("")
	}
}
