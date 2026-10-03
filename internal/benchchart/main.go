// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

// Command benchchart draws the README throughput chart from `go test -bench`
// output. It writes throughput-light.svg and throughput-dark.svg into the -out
// directory, one line per machine, in the order the machines are given.
//
// Usage, from the repository root:
//
//	go run ./internal/benchchart -out docs \
//		"Apple M4=bench/results/m4.txt" \
//		"AMD EPYC 9654P=bench/results/epyc9654p.txt" \
//		"Intel Xeon 8488C (Sapphire Rapids)=bench/results/sapphirerapids8488c.txt" \
//		"Intel Xeon 8375C (Ice Lake)=bench/results/icelake8375c.txt" \
//		"AWS Graviton4=bench/results/graviton4.txt" \
//		"AWS Graviton3=bench/results/graviton3.txt" \
//		"AWS Graviton2=bench/results/graviton2.txt"
//
// Each argument is label=path. From each file the program reads the
// BenchmarkCompare/size=N/impl=travcunn lines. A file without them falls back
// to the BenchmarkTiers/<tier>/N lines of the selected kernel: the tier named
// last in the file's "tiers on this machine: [...]" line, or, without that
// line, the tier with the highest 1 MiB median. The chart plots the median
// MB/s per size, converted to GB/s.
package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// A point is one median throughput at one input size.
type point struct {
	size int     // bytes
	gbps float64 // 10^9 bytes per second
}

// A series is one machine's line on the chart.
type series struct {
	label  string
	points []point // sorted by size
}

var (
	compareLine = regexp.MustCompile(`^BenchmarkCompare/size=(\d+)/impl=travcunn(?:-\d+)?\s+\d+\s+[\d.]+ ns/op\s+([\d.]+) MB/s`)
	tiersLine   = regexp.MustCompile(`^BenchmarkTiers/(\w+)/(\d+)(?:-\d+)?\s+\d+\s+[\d.]+ ns/op\s+([\d.]+) MB/s`)
	tiersList   = regexp.MustCompile(`tiers on this machine: \[([^\]]*)\]`)
)

const mib = 1 << 20

// parse extracts the selected kernel's median throughput per size from one
// `go test -bench` output.
func parse(r io.Reader) ([]point, error) {
	compare := map[int][]float64{}
	tiers := map[string]map[int][]float64{}
	selected := ""
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if m := compareLine.FindStringSubmatch(line); m != nil {
			size, mbps, err := sizeAndRate(m[1], m[2])
			if err != nil {
				return nil, err
			}
			compare[size] = append(compare[size], mbps)
			continue
		}
		if m := tiersLine.FindStringSubmatch(line); m != nil {
			size, mbps, err := sizeAndRate(m[2], m[3])
			if err != nil {
				return nil, err
			}
			if tiers[m[1]] == nil {
				tiers[m[1]] = map[int][]float64{}
			}
			tiers[m[1]][size] = append(tiers[m[1]][size], mbps)
			continue
		}
		if m := tiersList.FindStringSubmatch(line); m != nil {
			if names := strings.Fields(m[1]); len(names) > 0 {
				selected = names[len(names)-1]
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(compare) > 0 {
		return medians(compare), nil
	}
	if len(tiers) == 0 {
		return nil, errors.New("no BenchmarkCompare/size=N/impl=travcunn or BenchmarkTiers lines")
	}
	if tiers[selected] == nil {
		selected = ""
		best := -1.0
		for name, bySize := range tiers {
			if s, ok := bySize[mib]; ok && median(s) > best {
				selected, best = name, median(s)
			}
		}
		if selected == "" {
			return nil, errors.New("no tiers line and no 1 MiB BenchmarkTiers result to pick the selected kernel")
		}
	}
	return medians(tiers[selected]), nil
}

func sizeAndRate(size, rate string) (int, float64, error) {
	n, err := strconv.Atoi(size)
	if err != nil {
		return 0, 0, err
	}
	mbps, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		return 0, 0, err
	}
	return n, mbps, nil
}

// medians turns MB/s samples per size into sorted GB/s medians.
func medians(bySize map[int][]float64) []point {
	pts := make([]point, 0, len(bySize))
	for size, samples := range bySize {
		pts = append(pts, point{size, median(samples) / 1000})
	}
	slices.SortFunc(pts, func(a, b point) int { return cmp.Compare(a.size, b.size) })
	return pts
}

// median is the middle sample, or the mean of the two middle samples for an
// even count, as benchstat reports it.
func median(samples []float64) float64 {
	s := slices.Clone(samples)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// A theme is one color scheme. Series colors are assigned by argument position.
type theme struct {
	surface, text, textMuted, grid string
	colors                         []string
}

var (
	light = theme{
		surface: "#fcfcfb", text: "#0b0b0b", textMuted: "#52514e", grid: "#e6e5e1",
		colors: []string{"#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#008300", "#4a3aa7"},
	}
	dark = theme{
		surface: "#1a1a19", text: "#ffffff", textMuted: "#c3c2b7", grid: "#33332f",
		colors: []string{"#3987e5", "#d95926", "#199e70", "#c98500", "#d55181", "#008300", "#9085e9"},
	}
)

// maxSeries is the number of distinguishable series colors.
const maxSeries = 7

// Chart geometry in SVG user units.
//
//	+---------------------------------------------------------------+
//	| title                                                         |
//	| subtitle                                                      |
//	| GB/s                                                          |
//	|  120 +-----------------------------------+                    |
//	|      |                   ____o-----------o-- label            |
//	|      |         ____o----/                o-- label            |
//	|    0 +-----------------------------------+                    |
//	|       64 B  256 B  1 KiB ...        64 MiB  <- label space -> |
//	+---------------------------------------------------------------+
const (
	width      = 960
	height     = 480
	pad        = 16
	fontFamily = `ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`

	titleSize = 16
	textSize  = 13
	smallSize = 12

	yAxisSpace  = 40 // left of the plot, for y tick labels
	xAxisSpace  = 28 // below the plot, for x tick labels
	labelGap    = 16 // minimum vertical distance between direct labels
	markerR     = 4
	leaderReach = 22 // horizontal length of the leader from a line end to its label
)

var xTicks = []struct {
	size  int
	label string
}{
	{64, "64 B"}, {256, "256 B"}, {1 << 10, "1 KiB"}, {4 << 10, "4 KiB"},
	{64 << 10, "64 KiB"}, {1 << 20, "1 MiB"}, {64 << 20, "64 MiB"},
}

// textWidth estimates the rendered width of s, from typical sans-serif glyph
// advances as a fraction of the font size. It errs wide so text is not clipped.
func textWidth(s string, size float64) float64 {
	em := 0.0
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			em += 0.72
		case r >= '0' && r <= '9':
			em += 0.62
		case r >= 'a' && r <= 'z':
			em += 0.58
		case r == ' ':
			em += 0.3
		default:
			em += 0.38
		}
	}
	return em * size
}

// niceStep returns a 1, 2 or 5 times power-of-ten step that splits [0, max]
// into at most 7 intervals.
func niceStep(max float64) float64 {
	if max <= 0 {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(max/7)))
	for _, m := range []float64{1, 2, 5, 10} {
		if max/(m*mag) <= 7 {
			return m * mag
		}
	}
	return 10 * mag
}

// spread moves label positions apart so neighbors are at least gap units
// apart, keeping each group of collided labels centered on the mean of its
// wanted positions and inside [lo, hi]. want must be sorted ascending.
func spread(want []float64, gap, lo, hi float64) []float64 {
	type block struct {
		start, sum float64 // first label's position, sum of wanted positions
		n          int
	}
	var blocks []block
	for _, w := range want {
		blocks = append(blocks, block{w, w, 1})
		// Merge with the previous block while they overlap.
		for len(blocks) > 1 {
			b, prev := &blocks[len(blocks)-1], &blocks[len(blocks)-2]
			if prev.start+float64(prev.n)*gap <= b.start {
				break
			}
			prev.sum += b.sum
			prev.n += b.n
			blocks = blocks[:len(blocks)-1]
			prev.start = prev.sum/float64(prev.n) - float64(prev.n-1)*gap/2
		}
	}
	out := make([]float64, 0, len(want))
	for _, b := range blocks {
		for i := range b.n {
			out = append(out, b.start+float64(i)*gap)
		}
	}
	// Clamp into range, pushing neighbors along so the gap is kept.
	for i := range out {
		if i > 0 {
			out[i] = max(out[i], out[i-1]+gap)
		}
		out[i] = max(out[i], lo)
	}
	for i := len(out) - 1; i >= 0; i-- {
		if i < len(out)-1 {
			out[i] = min(out[i], out[i+1]-gap)
		}
		out[i] = min(out[i], hi)
	}
	return out
}

// render writes one SVG chart of the series in the given theme.
func render(w io.Writer, ss []series, th theme) error {
	if len(ss) > len(th.colors) {
		return fmt.Errorf("%d series, but only %d colors", len(ss), len(th.colors))
	}
	var b bytes.Buffer
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }

	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="100%%" role="img" aria-label="%s" font-family='%s'>`+"\n",
		width, height, esc("CRC-64/NVME throughput in GB/s against input size, one line per machine"), fontFamily)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`+"\n", width, height, th.surface)

	// Title and subtitle.
	y := float64(pad + titleSize)
	fmt.Fprintf(&b, `<text x="%d" y="%s" font-size="%d" font-weight="600" fill="%s">%s</text>`+"\n",
		pad, f(y), titleSize, th.text, esc("CRC-64/NVME throughput by input size, single core"))
	y += textSize + 8
	fmt.Fprintf(&b, `<text x="%d" y="%s" font-size="%d" fill="%s">%s</text>`+"\n",
		pad, f(y), textSize, th.textMuted, esc("benchstat median, this package's selected kernel on each machine"))

	// Plot area.
	plotTop := y + 40 // leaves room for the GB/s label above the top gridline
	plotBottom := float64(height - pad - xAxisSpace)
	plotLeft := float64(pad + yAxisSpace)
	// The direct labels sit right of the plot: marker, gap, leader, gap, text.
	labelSpace := 0.0
	for _, s := range ss {
		labelSpace = max(labelSpace, markerR+4+leaderReach+4+textWidth(s.label, smallSize))
	}
	plotRight := width - pad - labelSpace

	maxGBps := 0.0
	for _, s := range ss {
		for _, p := range s.points {
			maxGBps = max(maxGBps, p.gbps)
		}
	}
	step := niceStep(maxGBps)
	yMax := math.Ceil(maxGBps/step) * step
	yOf := func(v float64) float64 { return plotBottom - v/yMax*(plotBottom-plotTop) }
	lo, hi := math.Log2(float64(xTicks[0].size)), math.Log2(float64(xTicks[len(xTicks)-1].size))
	xOf := func(size int) float64 {
		return plotLeft + (math.Log2(float64(size))-lo)/(hi-lo)*(plotRight-plotLeft)
	}

	// Horizontal gridlines with y tick labels, then the axis unit.
	for v := 0.0; v <= yMax+step/2; v += step {
		gy := yOf(v)
		fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="1"/>`+"\n",
			f(plotLeft), f(gy), f(plotRight), f(gy), th.grid)
		fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="%d" text-anchor="end" fill="%s">%s</text>`+"\n",
			f(plotLeft-8), f(gy+4), smallSize, th.textMuted, strconv.FormatFloat(v, 'f', -1, 64))
	}
	fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="%d" text-anchor="end" fill="%s">GB/s</text>`+"\n",
		f(plotLeft-8), f(plotTop-14), smallSize, th.textMuted)

	// Axes: the y axis on the left, the x axis on the zero gridline.
	fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="1"/>`+"\n",
		f(plotLeft), f(plotTop), f(plotLeft), f(plotBottom), th.grid)
	for _, t := range xTicks {
		tx := xOf(t.size)
		fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="1"/>`+"\n",
			f(tx), f(plotBottom), f(tx), f(plotBottom+4), th.grid)
		fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="%d" text-anchor="middle" fill="%s">%s</text>`+"\n",
			f(tx), f(plotBottom+20), smallSize, th.textMuted, esc(t.label))
	}

	// Lines, then markers on top of every line.
	for i, s := range ss {
		pts := make([]string, len(s.points))
		for j, p := range s.points {
			pts[j] = f(xOf(p.size)) + "," + f(yOf(p.gbps))
		}
		fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>`+"\n",
			strings.Join(pts, " "), th.colors[i])
	}
	for i, s := range ss {
		for _, p := range s.points {
			fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%d" fill="%s" stroke="%s" stroke-width="2"/>`+"\n",
				f(xOf(p.size)), f(yOf(p.gbps)), markerR, th.colors[i], th.surface)
		}
	}

	// Direct labels at each line's right end, spread apart vertically, each
	// joined to its line end by a short leader in the series color.
	type end struct {
		i    int
		x, y float64
	}
	var ends []end
	for i, s := range ss {
		if len(s.points) > 0 {
			last := s.points[len(s.points)-1]
			ends = append(ends, end{i, xOf(last.size), yOf(last.gbps)})
		}
	}
	slices.SortStableFunc(ends, func(a, b end) int { return cmp.Compare(a.y, b.y) })
	want := make([]float64, len(ends))
	for k, e := range ends {
		want[k] = e.y
	}
	placed := spread(want, labelGap, plotTop, plotBottom)
	for k, e := range ends {
		x0 := e.x + markerR + 4
		x1 := x0 + leaderReach
		fmt.Fprintf(&b, `<path d="M%s %sL%s %sL%s %s" fill="none" stroke="%s" stroke-width="1"/>`+"\n",
			f(x0), f(e.y), f(x0+leaderReach/2), f(placed[k]), f(x1), f(placed[k]), th.colors[e.i])
		fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="%d" fill="%s">%s</text>`+"\n",
			f(x1+4), f(placed[k]+4), smallSize, th.text, esc(ss[e.i].label))
	}

	b.WriteString("</svg>\n")
	_, err := w.Write(b.Bytes())
	return err
}

func esc(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		panic(err) // strings.Builder never fails
	}
	return b.String()
}

// loadSeries parses label=path arguments in order.
func loadSeries(args []string) ([]series, error) {
	if len(args) == 0 {
		return nil, errors.New("no label=path arguments")
	}
	if len(args) > maxSeries {
		return nil, fmt.Errorf("%d machines given, at most %d fit the palette", len(args), maxSeries)
	}
	ss := make([]series, 0, len(args))
	for _, arg := range args {
		label, path, ok := strings.Cut(arg, "=")
		if !ok || label == "" || path == "" {
			return nil, fmt.Errorf("argument %q is not label=path", arg)
		}
		fh, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		pts, err := parse(fh)
		fh.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		ss = append(ss, series{label, pts})
	}
	return ss, nil
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("benchchart: ")
	out := flag.String("out", "docs", "directory for throughput-light.svg and throughput-dark.svg")
	flag.Parse()
	ss, err := loadSeries(flag.Args())
	if err != nil {
		log.Fatal(err)
	}
	for name, th := range map[string]theme{"throughput-light.svg": light, "throughput-dark.svg": dark} {
		var b bytes.Buffer
		if err := render(&b, ss, th); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(*out, name), b.Bytes(), 0o644); err != nil {
			log.Fatal(err)
		}
	}
}
