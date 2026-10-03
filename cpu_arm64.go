// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

package crc64nvme

import (
	"runtime"

	"golang.org/x/sys/cpu"
)

var availableTiers = detectTiers()

// detectTiers lists every tier this CPU supports, ordered so that the
// preferred kernel comes last (bestTier picks the last one with a kernel).
//
// Apple cores fuse PMULL with the EOR that follows it into one micro-op, so
// the PMULL kernel issues 2 uops per lane against 3 for EOR3. On an Apple M4
// the PMULL kernel measured within 1% of EOR3 at 4 KiB and 6.6% faster at 1 MiB.
// Cores without that fusion (Arm Neoverse V1 and V2) save a uop with EOR3 and
// prefer it. x/sys/cpu exposes no implementer ID, so GOOS stands in for
// "Apple core". Asahi Linux on Apple silicon therefore gets EOR3, which is
// correct and slightly slower.
func detectTiers() []tier {
	tiers := []tier{tierGeneric}
	hasPMULL := cpu.ARM64.HasPMULL
	hasEOR3 := cpu.ARM64.HasPMULL && cpu.ARM64.HasSHA3
	if runtime.GOOS == "darwin" {
		if hasEOR3 {
			tiers = append(tiers, tierEOR3)
		}
		if hasPMULL {
			tiers = append(tiers, tierPMULL)
		}
		return tiers
	}
	if hasPMULL {
		tiers = append(tiers, tierPMULL)
	}
	if hasEOR3 {
		tiers = append(tiers, tierEOR3)
	}
	return tiers
}

func init() {
	// Kernels register here, before the loop. bestTier is the last entry of
	// availableTiers that has a kernel registered.
	foldFuncs[tierPMULL] = foldPMULL
	foldFuncs[tierEOR3] = foldEOR3
	for i := len(availableTiers) - 1; i >= 0; i-- {
		if t := availableTiers[i]; t == tierGeneric || foldFuncs[t] != nil {
			bestTier = t
			break
		}
	}
}
