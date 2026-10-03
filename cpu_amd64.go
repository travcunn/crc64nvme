// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

package crc64nvme

import "golang.org/x/sys/cpu"

func cpuid(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)

// hasVPCLMULQDQ reads CPUID.(EAX=7,ECX=0):ECX[10] directly. x/sys/cpu only
// reports this bit when the OS enables AVX-512 state, which hides the VEX-256
// form on CPUs such as Zen 3 and Alder Lake that have VPCLMULQDQ without AVX-512.
func hasVPCLMULQDQ() bool {
	maxLeaf, _, _, _ := cpuid(0, 0)
	if maxLeaf < 7 {
		return false
	}
	_, _, ecx, _ := cpuid(7, 0)
	return ecx&(1<<10) != 0
}

var availableTiers = detectTiers()

func detectTiers() []tier {
	tiers := []tier{tierGeneric}
	if cpu.X86.HasSSE41 && cpu.X86.HasPCLMULQDQ {
		tiers = append(tiers, tierSSE)
	}
	if false && cpu.X86.HasAVX2 && cpu.X86.HasPCLMULQDQ && hasVPCLMULQDQ() { // enabled in Task 6
		tiers = append(tiers, tierAVX2)
	}
	if false && cpu.X86.HasAVX512F && cpu.X86.HasAVX512VL && cpu.X86.HasAVX512VPCLMULQDQ { // enabled in Task 7
		tiers = append(tiers, tierAVX512)
	}
	return tiers
}

func init() {
	foldFuncs[tierSSE] = foldSSE
	// Tasks 5 to 7 register kernels here, before the loop:
	// foldFuncs[tierSSE] = foldSSE, etc. bestTier is the last entry of
	// availableTiers that has a kernel registered.
	for i := len(availableTiers) - 1; i >= 0; i-- {
		if t := availableTiers[i]; t == tierGeneric || foldFuncs[t] != nil {
			bestTier = t
			break
		}
	}
}
