package ui

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/sys/unix"
)

// vizState holds the smoothed spectrum shown by the visualizer.
type vizState struct {
	level []float64 // smoothed bar heights, 0..1
	peak  []float64 // falling peak caps, 0..1
	hold  []int     // ticks a peak stays put before falling
	live  bool      // true when fed by real audio (vs idle/fallback)
	gain  float64   // slow auto-gain reference: recent loudest band
}

// vizBarW and vizGap set the bar geometry inside the visualizer box.
const (
	vizBarW = 2
	vizGap  = 1
)

// vizBandsFor returns how many bars fit in w columns.
func vizBandsFor(w int) int { return max((w+vizGap)/(vizBarW+vizGap), 1) }

// update folds a fresh spectrum frame into the smoothed state. A nil frame
// (paused / nothing playing) lets every bar fall.
func (v *vizState) update(frame []float64, n int) {
	if len(v.level) != n {
		v.level = make([]float64, n)
		v.peak = make([]float64, n)
		v.hold = make([]int, n)
	}
	v.live = frame != nil
	// Slow auto-gain so quiet masters still move and loud ones don't peg.
	var loud float64
	for _, f := range frame {
		loud = max(loud, f)
	}
	if loud > v.gain {
		v.gain += (loud - v.gain) * 0.3
	} else {
		v.gain *= 0.997
	}
	scale := 0.95 / max(v.gain, 0.55)
	for i := range n {
		target := 0.0
		if frame != nil {
			target = min(resampleAt(frame, i, n)*scale, 1)
		}
		cur := v.level[i]
		if target > cur {
			cur += (target - cur) * 0.65 // fast attack
		} else {
			cur += (target - cur) * 0.22 // slower release
		}
		v.level[i] = cur
		switch {
		case cur >= v.peak[i]:
			v.peak[i] = cur
			v.hold[i] = 8
		case v.hold[i] > 0:
			v.hold[i]--
		default:
			v.peak[i] = max(v.peak[i]-0.025, cur)
		}
	}
}

// resampleAt reads band i of n from a spectrum of arbitrary length.
func resampleAt(frame []float64, i, n int) float64 {
	if len(frame) == n {
		return frame[i]
	}
	lo := i * len(frame) / n
	hi := max((i+1)*len(frame)/n, lo+1)
	var m float64
	for j := lo; j < hi && j < len(frame); j++ {
		m = max(m, frame[j])
	}
	return m
}

// renderVisualizer draws the spectrum as vertical gradient bars with falling
// peak caps, w columns × h rows.
func renderVisualizer(t Theme, v *vizState, w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	n := vizBandsFor(w)
	used := n*vizBarW + (n-1)*vizGap
	leftPad := max((w-used)/2, 0)
	colors := vizRowColors(t, h)
	eighths := []rune(" ▁▂▃▄▅▆▇█")
	faint := lipgloss.NewStyle().Foreground(t.P.FgFaint)

	var sb strings.Builder
	for row := range h {
		fromBottom := h - 1 - row // 0 = bottom row
		var line strings.Builder
		line.WriteString(strings.Repeat(" ", leftPad))
		lit := false
		for b := range n {
			if b > 0 {
				line.WriteString(strings.Repeat(" ", vizGap))
			}
			lvl, pk := 0.0, 0.0
			if b < len(v.level) {
				lvl, pk = v.level[b], v.peak[b]
			}
			fill := lvl * float64(h) * 8 // in eighths of a cell
			cellFill := int(math.Round(fill)) - fromBottom*8
			ch := ' '
			switch {
			case cellFill >= 8:
				ch = '█'
			case cellFill > 0:
				ch = eighths[cellFill]
			}
			if ch == ' ' && pk > 0.02 {
				if int(pk*float64(h)) == fromBottom {
					ch = '▔'
				}
			}
			if ch != ' ' {
				lit = true
			}
			line.WriteString(strings.Repeat(string(ch), vizBarW))
		}
		text := line.String()
		switch {
		case lit:
			text = lipgloss.NewStyle().Foreground(colors[fromBottom]).Render(text)
		case fromBottom == 0:
			// Idle baseline so the box never looks empty.
			text = faint.Render(strings.Repeat(" ", leftPad) + baseline(n))
		}
		sb.WriteString(text)
		if row < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func baseline(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = strings.Repeat("▁", vizBarW)
	}
	return strings.Join(parts, strings.Repeat(" ", vizGap))
}

// vizRowColors interpolates the theme's 4-stop gradient bottom→top over h rows.
func vizRowColors(t Theme, h int) []lipgloss.TerminalColor {
	stops := gradColors(t)
	rgb := make([][3]float64, 0, len(stops))
	for _, s := range stops {
		if c, ok := hexRGB(s); ok {
			rgb = append(rgb, c)
		}
	}
	out := make([]lipgloss.TerminalColor, h)
	for i := range h {
		if len(rgb) < 2 {
			out[i] = stops[min(i*len(stops)/max(h, 1), len(stops)-1)]
			continue
		}
		f := float64(i) / float64(max(h-1, 1)) * float64(len(rgb)-1)
		k := min(int(f), len(rgb)-2)
		u := f - float64(k)
		a, b := rgb[k], rgb[k+1]
		out[i] = lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
			int(a[0]+(b[0]-a[0])*u), int(a[1]+(b[1]-a[1])*u), int(a[2]+(b[2]-a[2])*u)))
	}
	return out
}

func hexRGB(c lipgloss.TerminalColor) ([3]float64, bool) {
	s, ok := c.(lipgloss.Color)
	if !ok || len(s) != 7 || s[0] != '#' {
		return [3]float64{}, false
	}
	v, err := strconv.ParseUint(string(s[1:]), 16, 32)
	if err != nil {
		return [3]float64{}, false
	}
	return [3]float64{float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff)}, true
}

// cellAspect returns terminal cell height÷width in pixels (≈2 for most
// fonts), used to keep the square cover square. Falls back to 2.0.
func cellAspect() float64 {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return 2.0
	}
	cw := float64(ws.Xpixel) / float64(ws.Col)
	ch := float64(ws.Ypixel) / float64(ws.Row)
	a := ch / cw
	if a < 1 || a > 4 {
		return 2.0
	}
	return a
}
