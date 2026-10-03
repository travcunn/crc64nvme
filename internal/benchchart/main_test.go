// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func checkPoints(t *testing.T, got []point, want map[int]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d points %v, want %d sizes %v", len(got), got, len(want), want)
	}
	for i, p := range got {
		if i > 0 && got[i-1].size >= p.size {
			t.Errorf("points not sorted by size: %v", got)
		}
		w, ok := want[p.size]
		if !ok {
			t.Errorf("unexpected size %d", p.size)
			continue
		}
		if math.Abs(p.gbps-w) > 1e-9 {
			t.Errorf("size %d: got %v GB/s, want %v", p.size, p.gbps, w)
		}
	}
}

func TestParseCompareMedian(t *testing.T) {
	const in = `goos: darwin
pkg: github.com/travcunn/crc64nvme/bench
BenchmarkCompare/size=64/impl=travcunn-10   	193429585	         6.123 ns/op	10452.84 MB/s
BenchmarkCompare/size=64/impl=travcunn-10   	196495461	         6.087 ns/op	10000.00 MB/s
BenchmarkCompare/size=64/impl=travcunn-10   	198340864	         6.049 ns/op	12000.00 MB/s
BenchmarkCompare/size=64/impl=stdlib-10     	 15447187	        78.09 ns/op	  819.56 MB/s
BenchmarkCompare/size=4096/impl=travcunn-10 	 28911772	        42.27 ns/op	96000.00 MB/s
BenchmarkCompare/size=4096/impl=travcunn-10 	 28911772	        42.27 ns/op	94000.00 MB/s
BenchmarkCompare/size=4096/impl=travcunn-10 	 28911772	        42.27 ns/op	95000.00 MB/s
BenchmarkCompare/size=4096/impl=stdlib-10   	   640731	      1865 ns/op	 1500.00 MB/s
BenchmarkTiers/generic/4096-10              	   640731	      1865 ns/op	 2195.83 MB/s
PASS
`
	got, err := parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	checkPoints(t, got, map[int]float64{64: 10.45284, 4096: 95})
}

func TestParseEvenSampleCountAveragesMiddlePair(t *testing.T) {
	const in = `BenchmarkCompare/size=256/impl=travcunn-8   1   1 ns/op   1000 MB/s
BenchmarkCompare/size=256/impl=travcunn-8   1   1 ns/op   4000 MB/s
BenchmarkCompare/size=256/impl=travcunn-8   1   1 ns/op   2000 MB/s
BenchmarkCompare/size=256/impl=travcunn-8   1   1 ns/op   3000 MB/s
`
	got, err := parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	checkPoints(t, got, map[int]float64{256: 2.5})
}

func TestParseWithoutProcsSuffix(t *testing.T) {
	// With GOMAXPROCS=1, go test prints benchmark names without the -N suffix.
	const in = `BenchmarkCompare/size=1024/impl=travcunn   1   1 ns/op   7000 MB/s
`
	got, err := parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	checkPoints(t, got, map[int]float64{1024: 7})
}

const tiersFixture = `BenchmarkTiers/generic/4096-2   1   1 ns/op    1300 MB/s
BenchmarkTiers/pmull/4096-2     1   1 ns/op   23000 MB/s
BenchmarkTiers/pmull/4096-2     1   1 ns/op   24000 MB/s
BenchmarkTiers/pmull/4096-2     1   1 ns/op   25000 MB/s
BenchmarkTiers/eor3/4096-2      1   1 ns/op   33000 MB/s
BenchmarkTiers/eor3/4096-2      1   1 ns/op   34000 MB/s
BenchmarkTiers/eor3/4096-2      1   1 ns/op   35000 MB/s
BenchmarkTiers/generic/1048576-2   1   1 ns/op    1400 MB/s
BenchmarkTiers/pmull/1048576-2     1   1 ns/op   21000 MB/s
BenchmarkTiers/pmull/1048576-2     1   1 ns/op   22000 MB/s
BenchmarkTiers/pmull/1048576-2     1   1 ns/op   23000 MB/s
BenchmarkTiers/eor3/1048576-2      1   1 ns/op   40000 MB/s
BenchmarkTiers/eor3/1048576-2      1   1 ns/op   41000 MB/s
BenchmarkTiers/eor3/1048576-2      1   1 ns/op   42000 MB/s
`

func TestParseTiersFallbackUsesSelectedTier(t *testing.T) {
	// The tiers line names pmull last, so pmull is plotted even though eor3 is faster.
	in := "    tier_test.go:36: tiers on this machine: [generic eor3 pmull], best pmull\n" + tiersFixture
	got, err := parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	checkPoints(t, got, map[int]float64{4096: 24, 1 << 20: 22})
}

func TestParseTiersFallbackWithoutTiersLinePicksFastestAt1MiB(t *testing.T) {
	got, err := parse(strings.NewReader(tiersFixture))
	if err != nil {
		t.Fatal(err)
	}
	checkPoints(t, got, map[int]float64{4096: 34, 1 << 20: 41})
}

func TestParseTiersLineNamingUnmeasuredTierFallsBackToFastest(t *testing.T) {
	in := "tiers on this machine: [generic sse avx2 avx512], best avx512\n" + tiersFixture
	got, err := parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	checkPoints(t, got, map[int]float64{4096: 34, 1 << 20: 41})
}

func TestParseErrors(t *testing.T) {
	for name, in := range map[string]string{
		"empty":        "",
		"no matches":   "goos: linux\nBenchmarkOther/size=64-8   1   1 ns/op   10 MB/s\nPASS\n",
		"other impl":   "BenchmarkCompare/size=64/impl=stdlib-8   1   1 ns/op   10 MB/s\n",
		"no 1 MiB":     "BenchmarkTiers/sse/4096-8   1   1 ns/op   10 MB/s\n",
		"size too big": "BenchmarkCompare/size=99999999999999999999/impl=travcunn-8   1   1 ns/op   10 MB/s\n",
	} {
		if got, err := parse(strings.NewReader(in)); err == nil {
			t.Errorf("%s: got %v, want an error", name, got)
		}
	}
}

func TestSpread(t *testing.T) {
	const gap, lo, hi = 14.0, 0.0, 300.0
	for _, want := range [][]float64{
		{10, 100, 200},               // far apart: unchanged
		{100, 101, 102, 103},         // one cluster
		{50, 55, 200, 201, 202},      // two clusters
		{-20, -10, 0},                // pushed down from the top bound
		{295, 299, 300, 300},         // pushed up from the bottom bound
		{20, 30, 40, 41, 42, 120},    // cluster that grows into a neighbor
		{100, 100, 100, 100, 100},    // identical positions
		{0, 14, 28, 42},              // exactly gap apart: unchanged
		{60, 62, 64, 66, 68, 70, 72}, // seven labels, as in the README chart
	} {
		got := spread(want, gap, lo, hi)
		if len(got) != len(want) {
			t.Fatalf("spread(%v) = %v: length changed", want, got)
		}
		for i := range got {
			if got[i] < lo || got[i] > hi {
				t.Errorf("spread(%v) = %v: %v outside [%v, %v]", want, got, got[i], lo, hi)
			}
			if i > 0 && got[i]-got[i-1] < gap-1e-9 {
				t.Errorf("spread(%v) = %v: labels %d and %d closer than %v", want, got, i-1, i, gap)
			}
		}
	}
	if got := spread([]float64{10, 100, 200}, gap, lo, hi); fmt.Sprint(got) != "[10 100 200]" {
		t.Errorf("separated labels moved: %v", got)
	}
	// A cluster stays centered on the mean of its wanted positions.
	got := spread([]float64{100, 101, 102}, gap, lo, hi)
	if mean := (got[0] + got[1] + got[2]) / 3; math.Abs(mean-101) > 1e-9 {
		t.Errorf("cluster mean %v, want 101 (%v)", mean, got)
	}
}

func TestNiceStep(t *testing.T) {
	for _, c := range []struct{ max, want float64 }{
		{111.4, 20}, {56.9, 10}, {18.2, 5}, {7, 1}, {0, 1},
	} {
		if got := niceStep(c.max); got != c.want {
			t.Errorf("niceStep(%v) = %v, want %v", c.max, got, c.want)
		}
	}
}

func sevenSeries() []series {
	ss := make([]series, maxSeries)
	for i := range ss {
		ss[i] = series{
			label: fmt.Sprintf("Machine %d <&>", i),
			points: []point{
				{64, float64(i + 1)}, {256, float64(i + 2)}, {1 << 10, float64(i + 5)}, {4 << 10, float64(10 * i)},
				{64 << 10, float64(11 * i)}, {1 << 20, float64(11 * i)}, {64 << 20, float64(3 * i)},
			},
		}
	}
	return ss
}

func TestRender(t *testing.T) {
	ss := sevenSeries()
	for name, th := range map[string]theme{"light": light, "dark": dark} {
		var b bytes.Buffer
		if err := render(&b, ss, th); err != nil {
			t.Fatal(err)
		}
		svg := b.String()

		// Well-formed XML.
		dec := xml.NewDecoder(strings.NewReader(svg))
		for {
			if _, err := dec.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s: malformed SVG: %v", name, err)
			}
		}

		if n := strings.Count(svg, "<polyline "); n != len(ss) {
			t.Errorf("%s: %d polylines, want one per series (%d)", name, n, len(ss))
		}
		for i := range ss {
			if !regexp.MustCompile(`<polyline [^>]*stroke="` + th.colors[i] + `"`).MatchString(svg) {
				t.Errorf("%s: no polyline in series color %s", name, th.colors[i])
			}
			if !strings.Contains(svg, ">Machine "+fmt.Sprint(i)+" &lt;&amp;&gt;</text>") {
				t.Errorf("%s: label of series %d missing or unescaped", name, i)
			}
		}
		if n := strings.Count(svg, "<circle "); n != 7*len(ss) {
			t.Errorf("%s: %d markers, want %d", name, n, 7*len(ss))
		}
		for _, tick := range xTicks {
			if !strings.Contains(svg, ">"+tick.label+"</text>") {
				t.Errorf("%s: x tick label %q missing", name, tick.label)
			}
		}
		for _, want := range []string{
			`viewBox="0 0 960 480"`, `width="100%"`,
			`<rect width="960" height="480" fill="` + th.surface + `"/>`,
			">CRC-64/NVME throughput by input size, single core</text>",
			">benchstat median, this package&#39;s selected kernel on each machine</text>",
			">GB/s</text>",
		} {
			if !strings.Contains(svg, want) {
				t.Errorf("%s: SVG lacks %q", name, want)
			}
		}
		for _, banned := range []string{"<script", "@import", "href="} {
			if strings.Contains(svg, banned) {
				t.Errorf("%s: SVG contains %q", name, banned)
			}
		}
		// Text never uses a series color.
		for _, c := range th.colors {
			if regexp.MustCompile(`<text [^>]*fill="` + c + `"`).MatchString(svg) {
				t.Errorf("%s: text filled with series color %s", name, c)
			}
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	var a, b bytes.Buffer
	if err := render(&a, sevenSeries(), light); err != nil {
		t.Fatal(err)
	}
	if err := render(&b, sevenSeries(), light); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Error("two renders of the same data differ")
	}
}

func TestRenderRejectsTooManySeries(t *testing.T) {
	ss := append(sevenSeries(), series{label: "eighth", points: []point{{64, 1}}})
	if err := render(io.Discard, ss, light); err == nil {
		t.Error("render accepted 8 series with 7 colors")
	}
}

func TestLoadSeries(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("BenchmarkCompare/size=64/impl=travcunn-8   1   1 ns/op   5000 MB/s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	ss, err := loadSeries([]string{"Apple M4=" + file, "B=" + file})
	if err != nil {
		t.Fatal(err)
	}
	// Order follows the arguments, so colors follow argument position.
	if len(ss) != 2 || ss[0].label != "Apple M4" || ss[1].label != "B" || ss[0].points[0].gbps != 5 {
		t.Fatalf("loadSeries = %+v", ss)
	}

	eight := make([]string, maxSeries+1)
	for i := range eight {
		eight[i] = fmt.Sprintf("M%d=%s", i, file)
	}
	for name, args := range map[string][]string{
		"none":          nil,
		"eight":         eight,
		"no equals":     {file},
		"empty label":   {"=" + file},
		"empty path":    {"M="},
		"missing file":  {"M=" + filepath.Join(dir, "missing.txt")},
		"no benchmarks": {"M=" + empty},
	} {
		if _, err := loadSeries(args); err == nil {
			t.Errorf("%s: loadSeries(%q) succeeded", name, args)
		}
	}
}

// TestCommittedResults pins the medians the README tables quote to the
// committed results files, so the chart and the tables cannot drift apart.
func TestCommittedResults(t *testing.T) {
	sizes := []int{64, 256, 1 << 10, 4 << 10, 64 << 10, 1 << 20, 64 << 20}
	for _, c := range []struct {
		file string
		want []float64 // GB/s at sizes, rounded to 0.1
	}{
		{"m4.txt", []float64{10.6, 35.4, 71.6, 95.6, 111.4, 107.5, 67.5}},
		{"epyc9654p.txt", []float64{5.4, 16.6, 35.7, 50.3, 56.9, 55.1, 23.6}},
		{"sapphirerapids8488c.txt", []float64{7.1, 25.7, 61.5, 91.2, 99.5, 100.9, 23.3}},
		{"icelake8375c.txt", []float64{5.3, 16.3, 35.6, 47.5, 53.6, 50.5, 17.7}},
		{"graviton4.txt", []float64{5.5, 17.6, 26.2, 38.8, 44.2, 43.2, 27.4}},
		{"graviton3.txt", []float64{4.1, 14.6, 25.1, 33.3, 36.7, 33.4, 25.2}},
		{"graviton2.txt", []float64{2.0, 6.2, 12.6, 16.4, 18.0, 18.2, 17.1}},
		// Tiers-only files exercise the fallback: no tiers line, so the
		// fastest kernel at 1 MiB (pmull on the M4, avx512 on the EPYC).
		{"m4-tiers.txt", []float64{0, 0, 0, 95.8, 0, 110.3, 0}},
		{"epyc9654p-tiers.txt", []float64{0, 0, 0, 50.1, 0, 54.6, 0}},
	} {
		t.Run(c.file, func(t *testing.T) {
			f, err := os.Open(filepath.Join("..", "..", "bench", "results", c.file))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			got, err := parse(f)
			if err != nil {
				t.Fatal(err)
			}
			want := map[int]float64{}
			for i, v := range c.want {
				if v != 0 {
					want[sizes[i]] = v
				}
			}
			if len(got) != len(want) {
				t.Fatalf("got %d sizes %v, want %d", len(got), got, len(want))
			}
			for _, p := range got {
				if r := math.Round(p.gbps*10) / 10; r != want[p.size] {
					t.Errorf("size %d: %.1f GB/s (%.4f), want %.1f", p.size, r, p.gbps, want[p.size])
				}
			}
		})
	}
}
