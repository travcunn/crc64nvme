// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

package crc64nvme

import (
	"runtime"

	"golang.org/x/sys/cpu"
)

var availableTiers = detectTiers()

// detectTiers lists every tier this CPU supports, ordered so that the
// preferred kernel comes last (selectBestTier picks the last one with a kernel).
//
// Apple cores fuse PMULL with the EOR that follows it into one micro-op, so
// the PMULL kernel issues 2 uops per lane against 3 for EOR3. On an Apple M4
// the 16-accumulator PMULL kernel measured about 28% faster than EOR3 at
// 1 KiB, 52% at 4 KiB and 24% at 1 MiB.
// Cores without that fusion save a uop per lane with EOR3. On Arm Neoverse V1
// and V2 (AWS Graviton3 and Graviton4) EOR3 measured 1.4 times PMULL at 4 KiB
// and 1.6 to 1.8 times at 1 MiB. x/sys/cpu exposes no implementer ID, so GOOS
// stands in for "Apple core". Asahi Linux on Apple silicon therefore gets
// EOR3, which is correct but gives up the 24 to 52% measured above.
func detectTiers() []tier {
	hasPMULL := cpu.ARM64.HasPMULL
	hasEOR3 := hasPMULL && cpu.ARM64.HasSHA3
	appleCore := runtime.GOOS == "darwin" || runtime.GOOS == "ios"

	tiers := []tier{tierGeneric}
	if hasEOR3 && appleCore {
		tiers = append(tiers, tierEOR3)
	}
	if hasPMULL {
		tiers = append(tiers, tierPMULL)
	}
	if hasEOR3 && !appleCore {
		tiers = append(tiers, tierEOR3)
	}
	return tiers
}

func init() {
	// Kernels register before selectBestTier reads foldFuncs.
	foldFuncs[tierPMULL] = foldPMULL
	foldFuncs[tierEOR3] = foldEOR3
	bestTier = selectBestTier(availableTiers)
}
