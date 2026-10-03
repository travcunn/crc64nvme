// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

package crc64nvme

// tier identifies a kernel. Kernels register in the init function of
// cpu_amd64.go or cpu_arm64.go, which also sets bestTier. Tests override
// bestTier directly.
type tier uint8

const (
	tierGeneric tier = iota
	tierSSE
	tierAVX2
	tierAVX512
	tierPMULL
	tierEOR3
	numTiers
)

var tierNames = [numTiers]string{"generic", "sse", "avx2", "avx512", "pmull", "eor3"}

func (t tier) String() string { return tierNames[t] }

// foldFunc is the kernel contract: len(p) is a multiple of 16 and at least 16.
type foldFunc func(crc uint64, p []byte) uint64

// foldFuncs is filled by cpu_*.go for the tiers this binary and CPU support.
var foldFuncs [numTiers]foldFunc

// bestTier is the fastest tier available, chosen at init by cpu_amd64.go or
// cpu_arm64.go.
var bestTier = tierGeneric

// selectBestTier returns the last entry of tiers that can run: tierGeneric or
// a tier with a kernel registered in foldFuncs. detectTiers orders tiers so
// the preferred kernel comes last.
func selectBestTier(tiers []tier) tier {
	for i := len(tiers) - 1; i >= 0; i-- {
		if t := tiers[i]; t == tierGeneric || foldFuncs[t] != nil {
			return t
		}
	}
	return tierGeneric
}

// avx512Min is the smallest input routed to the AVX-512 kernel. Smaller
// inputs on an AVX-512 CPU use the AVX2 kernel. 256 is the AVX-512 block size,
// and on Zen 4 the AVX-512 kernel is faster at every size from there up.
var avx512Min = 256

func update(crc uint64, p []byte) uint64 {
	if len(p) < 16 || bestTier == tierGeneric {
		return updateGeneric(crc, p)
	}
	n := len(p) &^ 15
	t := bestTier
	if t == tierAVX512 && n < avx512Min {
		t = tierAVX2
	}
	crc = foldFuncs[t](crc, p[:n])
	return updateGeneric(crc, p[n:])
}
