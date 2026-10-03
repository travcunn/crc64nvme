// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

#include "textflag.h"

// Placeholder body, replaced by the real kernel in a later task. Returns crc
// unchanged so a stray call fails the oracle comparison instead of crashing.

// func foldSSE(crc uint64, p []byte) uint64
TEXT ·foldSSE(SB), NOSPLIT, $0-40
	MOVQ crc+0(FP), AX
	MOVQ AX, ret+32(FP)
	RET
