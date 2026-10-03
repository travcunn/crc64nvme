// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

package crc64nvme

// tier identifies a kernel. Later tasks add kernels and the CPU detection
// that sets bestTier at init. Tests override bestTier directly.
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

// bestTier is the fastest tier available, chosen at init by cpu_*.go.
var bestTier = tierGeneric

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
