// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

#include "textflag.h"

// acc = fold(acc) ^ data. Each PMULL is immediately followed by an EOR into
// the same register so Apple cores can fuse the pair into one micro-op.
#define FOLD_LANE(acc, data) \
	VPMULL  V8.D1, acc.D1, V9.Q1      \
	VEOR    data.B16, V9.B16, V9.B16  \
	VPMULL2 V8.D2, acc.D2, acc.Q1     \
	VEOR    V9.B16, acc.B16, acc.B16

// V7 ^= fold(acc) with the constant pair at off(R3).
#define COMBINE_LANE(acc, off) \
	ADD     $off, R3, R4              \
	VLD1    (R4), [V8.B16]            \
	VPMULL  V8.D1, acc.D1, V9.Q1      \
	VEOR    V7.B16, V9.B16, V9.B16    \
	VPMULL2 V8.D2, acc.D2, acc.Q1     \
	VEOR    V9.B16, acc.B16, V7.B16

// func foldPMULL(crc uint64, p []byte) uint64
TEXT ·foldPMULL(SB), NOSPLIT, $0-40
	MOVD crc+0(FP), R0
	MOVD p_base+8(FP), R1
	MOVD p_len+16(FP), R2
	MOVD $·foldK(SB), R3
	MVN  R0, R0
	VEOR V9.B16, V9.B16, V9.B16
	VMOV R0, V9.D[0]               // V9 = ^crc in the low qword
	CMP  $128, R2
	BLO  single

	VLD1.P 64(R1), [V0.B16, V1.B16, V2.B16, V3.B16]
	VLD1.P 64(R1), [V4.B16, V5.B16, V6.B16, V7.B16]
	VEOR   V9.B16, V0.B16, V0.B16
	SUB    $128, R2, R2
	ADD    $112, R3, R4
	VLD1   (R4), [V8.B16]          // d=128
	CMP    $128, R2
	BLO    combine

loop:
	VLD1.P 64(R1), [V11.B16, V12.B16, V13.B16, V14.B16]
	FOLD_LANE(V0, V11)
	FOLD_LANE(V1, V12)
	FOLD_LANE(V2, V13)
	FOLD_LANE(V3, V14)
	VLD1.P 64(R1), [V11.B16, V12.B16, V13.B16, V14.B16]
	FOLD_LANE(V4, V11)
	FOLD_LANE(V5, V12)
	FOLD_LANE(V6, V13)
	FOLD_LANE(V7, V14)
	SUB    $128, R2, R2
	CMP    $128, R2
	BHS    loop

combine:
	// Lane i is 16*(7-i) bytes ahead of lane 7: constant entry (7-i)-1.
	COMBINE_LANE(V0, 96)           // d=112
	COMBINE_LANE(V1, 80)           // d=96
	COMBINE_LANE(V2, 64)           // d=80
	COMBINE_LANE(V3, 48)           // d=64
	COMBINE_LANE(V4, 32)           // d=48
	COMBINE_LANE(V5, 16)           // d=32
	COMBINE_LANE(V6, 0)            // d=16
	VMOV V7.B16, V0.B16
	B    tail

single:
	VLD1.P 16(R1), [V0.B16]
	VEOR   V9.B16, V0.B16, V0.B16
	SUB    $16, R2, R2

tail:
	CBZ  R2, reduce
	VLD1 (R3), [V8.B16]            // d=16

tailloop:
	VLD1.P 16(R1), [V11.B16]
	FOLD_LANE(V0, V11)
	SUB  $16, R2, R2
	CBNZ R2, tailloop

reduce:
	MOVD   $·reduceK(SB), R3
	VLD1   (R3), [V8.B16]          // K127 low
	VPMULL V8.D1, V0.D1, V1.Q1     // A
	VMOV   V0.D[1], R4             // R.hi
	VMOV   V1.D[0], R5             // A.lo
	EOR    R4, R5, R5              // T.lo
	VMOV   V1.D[1], R6             // T.hi
	VMOV   R5, V2.D[0]
	ADD    $16, R3, R4
	VLD1   (R4), [V8.B16]          // MU
	VPMULL V8.D1, V2.D1, V3.Q1
	VMOV   V3.D[0], R7             // t2
	VMOV   R7, V3.D[0]
	ADD    $32, R3, R4
	VLD1   (R4), [V8.B16]          // POLY
	VPMULL V8.D1, V3.D1, V4.Q1     // D
	VMOV   V4.D[1], R8             // D.hi
	EOR    R6, R8, R0
	EOR    R7, R0, R0
	MVN    R0, R0
	MOVD   R0, ret+32(FP)
	RET
