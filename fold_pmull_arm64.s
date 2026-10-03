// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

#include "textflag.h"

// The main loop folds 256-byte blocks into 16 accumulators. Apple cores fuse
// each PMULL with the EOR that follows it, so a lane costs 2 uops and the
// SIMD pipes retire about 4 per cycle. With 8 accumulators each lane's
// PMULL->EOR->PMULL chain (about 6 cycles) is revisited before it completes
// and the loop is latency-bound. 16 accumulators double the window per chain
// and make it throughput-bound. The message of commit 74ae622 has the Apple M4
// measurements against the 8-accumulator kernel.
//
// After the 16-lane loop, lanes 0..7 are folded 128 bytes forward and XORed
// with lanes 8..15 (8 independent folds with one constant), and the results
// stay in V0..V7. That leaves an ordinary 8-lane state, so the remainder
// below 256 bytes runs through the 8-lane loop and the tail never folds more
// than 7 single lanes.
//
//	len >= 256:  16-lane loop -> fold to 8 lanes -> [loop] -> combine -.
//	len >= 128:  8-lane load ---------------------> [loop] -> combine -+
//	len <  128:  single lane ------------------------------------------+-> tail -> reduce
//
// [loop] is the 8-lane loop, which runs zero or more times.
//
// Registers: V0..V15 accumulators, V16 constant pair, V17..V18 temps,
// V20..V23 data.

// acc = fold(acc) ^ data. Each PMULL is immediately followed by an EOR into
// the same register so Apple cores can fuse the pair into one micro-op.
#define FOLD_LANE(acc, data) \
	VPMULL  V16.D1, acc.D1, V17.Q1    \
	VEOR    data.B16, V17.B16, V17.B16 \
	VPMULL2 V16.D2, acc.D2, acc.Q1    \
	VEOR    V17.B16, acc.B16, acc.B16

// V7 ^= fold(acc) with the constant pair at off(R3). lo^hi is formed in V18
// so only one EOR per lane is on the V7 dependency chain. The PMULL2 is
// followed by an EOR into its own destination so Apple cores can fuse that
// pair.
#define COMBINE_LANE(acc, off) \
	ADD     $off, R3, R4              \
	VLD1    (R4), [V16.B16]           \
	VPMULL  V16.D1, acc.D1, V17.Q1    \
	VPMULL2 V16.D2, acc.D2, V18.Q1    \
	VEOR    V17.B16, V18.B16, V18.B16 \
	VEOR    V18.B16, V7.B16, V7.B16

// func foldPMULL(crc uint64, p []byte) uint64
TEXT ·foldPMULL(SB), NOSPLIT, $0-40
	MOVD crc+0(FP), R0
	MOVD p_base+8(FP), R1
	MOVD p_len+16(FP), R2
	MOVD $·foldK(SB), R3
	MVN  R0, R0
	VEOR V17.B16, V17.B16, V17.B16
	VMOV R0, V17.D[0]              // V17 = ^crc in the low qword
	CMP  $256, R2
	BHS  lanes16
	CMP  $128, R2
	BHS  lanes8

	VLD1.P 16(R1), [V0.B16]
	VEOR   V17.B16, V0.B16, V0.B16
	SUB    $16, R2, R2
	B      tail

lanes16:
	VLD1.P 64(R1), [V0.B16, V1.B16, V2.B16, V3.B16]
	VLD1.P 64(R1), [V4.B16, V5.B16, V6.B16, V7.B16]
	VLD1.P 64(R1), [V8.B16, V9.B16, V10.B16, V11.B16]
	VLD1.P 64(R1), [V12.B16, V13.B16, V14.B16, V15.B16]
	VEOR   V17.B16, V0.B16, V0.B16
	SUB    $256, R2, R2
	ADD    $240, R3, R4
	VLD1   (R4), [V16.B16]         // d=256 K(2111), K(2047)
	CMP    $256, R2
	BLO    fold16to8

	// Loop entries are aligned so performance does not depend on link layout.
	PCALIGN $16
loop16:
	VLD1.P 64(R1), [V20.B16, V21.B16, V22.B16, V23.B16]
	FOLD_LANE(V0, V20)
	FOLD_LANE(V1, V21)
	FOLD_LANE(V2, V22)
	FOLD_LANE(V3, V23)
	VLD1.P 64(R1), [V20.B16, V21.B16, V22.B16, V23.B16]
	FOLD_LANE(V4, V20)
	FOLD_LANE(V5, V21)
	FOLD_LANE(V6, V22)
	FOLD_LANE(V7, V23)
	VLD1.P 64(R1), [V20.B16, V21.B16, V22.B16, V23.B16]
	FOLD_LANE(V8, V20)
	FOLD_LANE(V9, V21)
	FOLD_LANE(V10, V22)
	FOLD_LANE(V11, V23)
	VLD1.P 64(R1), [V20.B16, V21.B16, V22.B16, V23.B16]
	FOLD_LANE(V12, V20)
	FOLD_LANE(V13, V21)
	FOLD_LANE(V14, V22)
	FOLD_LANE(V15, V23)
	SUB    $256, R2, R2
	CMP    $256, R2
	BHS    loop16

fold16to8:
	// Lane i+8 is 128 bytes ahead of lane i: V(i) = fold(V(i), 128) ^ V(i+8).
	ADD    $112, R3, R4
	VLD1   (R4), [V16.B16]         // d=128 K(1087), K(1023)
	FOLD_LANE(V0, V8)
	FOLD_LANE(V1, V9)
	FOLD_LANE(V2, V10)
	FOLD_LANE(V3, V11)
	FOLD_LANE(V4, V12)
	FOLD_LANE(V5, V13)
	FOLD_LANE(V6, V14)
	FOLD_LANE(V7, V15)
	CMP    $128, R2
	BHS    loop8
	B      combine8

lanes8:
	VLD1.P 64(R1), [V0.B16, V1.B16, V2.B16, V3.B16]
	VLD1.P 64(R1), [V4.B16, V5.B16, V6.B16, V7.B16]
	VEOR   V17.B16, V0.B16, V0.B16
	SUB    $128, R2, R2
	ADD    $112, R3, R4
	VLD1   (R4), [V16.B16]         // d=128 K(1087), K(1023)
	CMP    $128, R2
	BLO    combine8

	PCALIGN $16
loop8:
	VLD1.P 64(R1), [V20.B16, V21.B16, V22.B16, V23.B16]
	FOLD_LANE(V0, V20)
	FOLD_LANE(V1, V21)
	FOLD_LANE(V2, V22)
	FOLD_LANE(V3, V23)
	VLD1.P 64(R1), [V20.B16, V21.B16, V22.B16, V23.B16]
	FOLD_LANE(V4, V20)
	FOLD_LANE(V5, V21)
	FOLD_LANE(V6, V22)
	FOLD_LANE(V7, V23)
	SUB    $128, R2, R2
	CMP    $128, R2
	BHS    loop8

combine8:
	// Lane i is 16*(7-i) bytes ahead of lane 7: constant entry (7-i)-1.
	COMBINE_LANE(V0, 96)           // d=112
	COMBINE_LANE(V1, 80)           // d=96
	COMBINE_LANE(V2, 64)           // d=80
	COMBINE_LANE(V3, 48)           // d=64
	COMBINE_LANE(V4, 32)           // d=48
	COMBINE_LANE(V5, 16)           // d=32
	COMBINE_LANE(V6, 0)            // d=16
	VMOV V7.B16, V0.B16

tail:
	CBZ  R2, reduce
	VLD1 (R3), [V16.B16]           // d=16

	PCALIGN $16
tailloop:
	VLD1.P 16(R1), [V20.B16]
	FOLD_LANE(V0, V20)
	SUB  $16, R2, R2
	CBNZ R2, tailloop

reduce:
	MOVD   $·reduceK(SB), R3
	VLD1   (R3), [V16.B16]         // K127 low
	VPMULL V16.D1, V0.D1, V1.Q1    // A
	VMOV   V0.D[1], R4             // R.hi
	VMOV   V1.D[0], R5             // A.lo
	EOR    R4, R5, R5              // T.lo
	VMOV   V1.D[1], R6             // T.hi
	VMOV   R5, V2.D[0]
	ADD    $16, R3, R4
	VLD1   (R4), [V16.B16]         // MU
	VPMULL V16.D1, V2.D1, V3.Q1
	VMOV   V3.D[0], R7             // t2
	ADD    $32, R3, R4
	VLD1   (R4), [V16.B16]         // POLY
	VPMULL V16.D1, V3.D1, V4.Q1    // D
	VMOV   V4.D[1], R8             // D.hi
	EOR    R6, R8, R0
	EOR    R7, R0, R0
	MVN    R0, R0
	MOVD   R0, ret+32(FP)
	RET
