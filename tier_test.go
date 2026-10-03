// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

package crc64nvme

import (
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/travcunn/crc64nvme/internal/model"
)

// forEachTier runs fn once per tier this machine can execute, with bestTier set
// and avx512Min lowered so the AVX-512 kernel also sees small inputs.
func forEachTier(t *testing.T, fn func(t *testing.T)) {
	t.Helper()
	oldTier, oldMin := bestTier, avx512Min
	t.Cleanup(func() { bestTier, avx512Min = oldTier, oldMin })
	for _, tr := range availableTiers {
		bestTier = tr
		avx512Min = 16
		t.Run(tr.String(), fn)
	}
}

// TestAvailableTiersListed logs the detected tiers. When CRC64NVME_EXPECT_TIERS
// is set to a comma-separated list of tier names, it also fails unless every
// named tier was detected, so CI catches a runner whose detection regressed.
func TestAvailableTiersListed(t *testing.T) {
	if len(availableTiers) == 0 || availableTiers[0] != tierGeneric {
		t.Fatalf("availableTiers = %v", availableTiers)
	}
	t.Logf("tiers on this machine: %v, best %v", availableTiers, bestTier)
	for _, tr := range availableTiers[1:] {
		if foldFuncs[tr] == nil {
			t.Errorf("tier %v listed without a kernel", tr)
		}
	}
	if expect := os.Getenv("CRC64NVME_EXPECT_TIERS"); expect != "" {
		for _, name := range strings.Split(expect, ",") {
			name = strings.TrimSpace(name)
			if !slices.ContainsFunc(availableTiers, func(tr tier) bool { return tr.String() == name }) {
				t.Errorf("CRC64NVME_EXPECT_TIERS names %q, which is not in availableTiers %v", name, availableTiers)
			}
		}
	}
}

func TestSelectBestTier(t *testing.T) {
	old := foldFuncs
	t.Cleanup(func() { foldFuncs = old })
	stub := func(crc uint64, _ []byte) uint64 { return crc }
	foldFuncs = [numTiers]foldFunc{}
	foldFuncs[tierSSE] = stub
	foldFuncs[tierPMULL] = stub
	foldFuncs[tierEOR3] = stub
	for _, c := range []struct {
		tiers []tier
		want  tier
	}{
		{nil, tierGeneric},
		{[]tier{tierGeneric}, tierGeneric},
		{[]tier{tierGeneric, tierSSE}, tierSSE},
		{[]tier{tierGeneric, tierSSE, tierAVX2}, tierSSE},
		{[]tier{tierGeneric, tierAVX2, tierAVX512}, tierGeneric},
		{[]tier{tierGeneric, tierEOR3, tierPMULL}, tierPMULL},
		{[]tier{tierGeneric, tierPMULL, tierEOR3}, tierEOR3},
	} {
		if got := selectBestTier(c.tiers); got != c.want {
			t.Errorf("selectBestTier(%v) = %v, want %v", c.tiers, got, c.want)
		}
	}
}

// TestArm64TierSelection pins the arm64 preference: PMULL on Apple cores
// (darwin, ios), EOR3 elsewhere when both kernels are available.
func TestArm64TierSelection(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("arm64 only")
	}
	if !slices.Contains(availableTiers, tierPMULL) || !slices.Contains(availableTiers, tierEOR3) {
		t.Skipf("needs both pmull and eor3, have %v", availableTiers)
	}
	want := tierEOR3
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		want = tierPMULL
	}
	if bestTier != want {
		t.Fatalf("GOOS %s, tiers %v: bestTier = %v, want %v", runtime.GOOS, availableTiers, bestTier, want)
	}
}

func TestTiersMatchOracle(t *testing.T) {
	rng := rand.NewChaCha8([32]byte{4})
	buf := make([]byte, 2048+16)
	rng.Read(buf)
	forEachTier(t, func(t *testing.T) {
		for size := 0; size <= 2048; size++ {
			for off := 0; off < 16; off++ {
				p := buf[off : off+size]
				crc0 := rng.Uint64()
				if got, want := Update(crc0, p), oracle(crc0, p); got != want {
					t.Fatalf("size %d off %d: got %#x want %#x", size, off, got, want)
				}
			}
		}
	})
}

func TestTiersLargeSizes(t *testing.T) {
	type input struct{ size, off int }
	inputs := []input{
		{4096, 0}, {8192, 0}, {65536, 0},
		{1 << 20, 0}, {1 << 20, 1}, {1 << 20, 15},
		{1048583, 0}, {1048583, 1}, {1048583, 15},
		{4 << 20, 0}, {4194319, 0},
	}
	if !testing.Short() {
		inputs = append(inputs, input{64 << 20, 0})
	}
	n := 0
	for _, in := range inputs {
		n = max(n, in.off+in.size)
	}
	buf := make([]byte, n)
	rand.NewChaCha8([32]byte{5}).Read(buf)
	forEachTier(t, func(t *testing.T) {
		for _, in := range inputs {
			p := buf[in.off : in.off+in.size]
			if got, want := Checksum(p), oracle(0, p); got != want {
				t.Fatalf("size %d off %d: got %#x want %#x", in.size, in.off, got, want)
			}
		}
	})
}

func TestKernelsMatchModel(t *testing.T) {
	// Every kernel must equal the model at the kernel boundary (16-byte multiples).
	rng := rand.NewChaCha8([32]byte{6})
	buf := make([]byte, 1024)
	rng.Read(buf)
	for _, tr := range availableTiers[1:] {
		t.Run(tr.String(), func(t *testing.T) {
			for size := 16; size <= 1024; size += 16 {
				crc0 := rng.Uint64()
				got := foldFuncs[tr](crc0, buf[:size])
				if want := model.Fold(crc0, buf[:size], 8); got != want {
					t.Fatalf("size %d: kernel %#x model %#x oracle %#x", size, got, want, oracle(crc0, buf[:size]))
				}
			}
		})
	}
}

func TestStreamingAllTiers(t *testing.T) {
	src := rand.NewChaCha8([32]byte{7})
	data := make([]byte, 50000)
	src.Read(data)
	rng := rand.New(src)
	want := oracle(0, data)
	forEachTier(t, func(t *testing.T) {
		for _, chunk := range []int{1, 15, 16, 17, 127, 128, 129, 255, 256, 257, 1023, 1024, 4097} {
			h := New()
			for i := 0; i < len(data); i += chunk {
				end := min(i+chunk, len(data))
				h.Write(data[i:end])
			}
			if got := h.Sum64(); got != want {
				t.Errorf("chunk %d: got %#x want %#x", chunk, got, want)
			}
		}
		// random split points
		for n := 0; n < 200; n++ {
			h := New()
			i := 0
			for i < len(data) {
				end := min(i+rng.IntN(3000), len(data))
				h.Write(data[i:end])
				i = end
			}
			if h.Sum64() != want {
				t.Fatal("random split mismatch")
			}
		}
	})
}

func TestAVX512Threshold(t *testing.T) {
	has := false
	for _, tr := range availableTiers {
		has = has || tr == tierAVX512
	}
	if !has {
		t.Skip("no AVX-512")
	}
	oldTier, oldMin := bestTier, avx512Min
	t.Cleanup(func() { bestTier, avx512Min = oldTier, oldMin })
	bestTier = tierAVX512
	buf := make([]byte, 4096)
	for i := range buf {
		buf[i] = byte(i * 7)
	}
	for _, threshold := range []int{16, 256, 1024, 4096} {
		avx512Min = threshold
		for _, size := range []int{threshold - 16, threshold - 1, threshold, threshold + 1, threshold + 16, threshold + 17} {
			if size < 0 || size > len(buf) {
				continue
			}
			if got, want := Checksum(buf[:size]), oracle(0, buf[:size]); got != want {
				t.Fatalf("threshold %d size %d: got %#x want %#x", threshold, size, got, want)
			}
		}
	}
}
