// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

package crc64nvme

import "golang.org/x/sys/cpu"

var availableTiers = detectTiers()

func detectTiers() []tier {
	tiers := []tier{tierGeneric}
	if cpu.ARM64.HasPMULL {
		tiers = append(tiers, tierPMULL)
	}
	if false && cpu.ARM64.HasPMULL && cpu.ARM64.HasSHA3 { // enabled in Task 9
		tiers = append(tiers, tierEOR3)
	}
	return tiers
}

func init() {
	// Kernels register here, before the loop. bestTier is the last entry of
	// availableTiers that has a kernel registered.
	foldFuncs[tierPMULL] = foldPMULL
	for i := len(availableTiers) - 1; i >= 0; i-- {
		if t := availableTiers[i]; t == tierGeneric || foldFuncs[t] != nil {
			bestTier = t
			break
		}
	}
}
