// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

#include "textflag.h"

// Fold one accumulator by the pair in X8 and XOR in 16 bytes at off(SI).
#define FOLD_LANE(acc, off) \
	MOVOU     acc, X9            \
	PCLMULQDQ $0x00, X8, X9      \
	PCLMULQDQ $0x11, X8, acc     \
	PXOR      X9, acc            \
	MOVOU     off(SI), X10       \
	PXOR      X10, acc

// Fold acc by the pair at koff(DX) and XOR the result into X7.
#define COMBINE_LANE(acc, koff) \
	MOVOU     koff(DX), X8       \
	MOVOU     acc, X9            \
	PCLMULQDQ $0x00, X8, X9      \
	PCLMULQDQ $0x11, X8, acc     \
	PXOR      X9, X7             \
	PXOR      acc, X7

// func foldSSE(crc uint64, p []byte) uint64
TEXT ·foldSSE(SB), NOSPLIT, $0-40
	MOVQ crc+0(FP), AX
	MOVQ p_base+8(FP), SI
	MOVQ p_len+16(FP), CX
	LEAQ ·foldK(SB), DX
	NOTQ AX
	MOVQ AX, X9              // X9 = ^crc in the low qword, zero above
	CMPQ CX, $128
	JB   single

	MOVOU 0(SI), X0
	MOVOU 16(SI), X1
	MOVOU 32(SI), X2
	MOVOU 48(SI), X3
	MOVOU 64(SI), X4
	MOVOU 80(SI), X5
	MOVOU 96(SI), X6
	MOVOU 112(SI), X7
	PXOR  X9, X0
	ADDQ  $128, SI
	SUBQ  $128, CX
	MOVOU 112(DX), X8        // d=128: K(1087), K(1023)
	CMPQ  CX, $128
	JB    combine

	// Loop entries are aligned so performance does not depend on link layout.
	PCALIGN $32
loop:
	FOLD_LANE(X0, 0)
	FOLD_LANE(X1, 16)
	FOLD_LANE(X2, 32)
	FOLD_LANE(X3, 48)
	FOLD_LANE(X4, 64)
	FOLD_LANE(X5, 80)
	FOLD_LANE(X6, 96)
	FOLD_LANE(X7, 112)
	ADDQ $128, SI
	SUBQ $128, CX
	CMPQ CX, $128
	JAE  loop

combine:
	// Lane i is 16*(7-i) bytes ahead of lane 7: constant entry (7-i)-1.
	COMBINE_LANE(X0, 96)     // d=112
	COMBINE_LANE(X1, 80)     // d=96
	COMBINE_LANE(X2, 64)     // d=80
	COMBINE_LANE(X3, 48)     // d=64
	COMBINE_LANE(X4, 32)     // d=48
	COMBINE_LANE(X5, 16)     // d=32
	COMBINE_LANE(X6, 0)      // d=16
	MOVOU X7, X0
	JMP   tail

single:
	MOVOU 0(SI), X0
	PXOR  X9, X0
	ADDQ  $16, SI
	SUBQ  $16, CX

tail:
	TESTQ CX, CX
	JZ    reduce
	MOVOU 0(DX), X8          // d=16: K(191), K(127)

	PCALIGN $32
tailloop:
	FOLD_LANE(X0, 0)
	ADDQ $16, SI
	SUBQ $16, CX
	JNZ  tailloop

reduce:
	LEAQ      ·reduceK(SB), DX
	MOVOU     0(DX), X8          // K127 in the low qword
	MOVOU     X0, X1
	PCLMULQDQ $0x00, X8, X1      // X1 = A = clmul(R.lo, K127)
	PSRLDQ    $8, X0             // X0.lo = R.hi
	PXOR      X0, X1             // X1 = T: lo = A.lo ^ R.hi, hi = A.hi
	MOVOU     16(DX), X8         // MU
	MOVOU     X1, X2
	PCLMULQDQ $0x00, X8, X2      // X2.lo = t2
	MOVOU     32(DX), X8         // POLY
	MOVOU     X2, X3
	PCLMULQDQ $0x00, X8, X3      // X3 = D = clmul(t2, POLY)
	PEXTRQ    $1, X1, AX         // T.hi
	PEXTRQ    $1, X3, BX         // D.hi
	XORQ      BX, AX
	MOVQ      X2, BX             // t2
	XORQ      BX, AX
	NOTQ      AX
	MOVQ      AX, ret+32(FP)
	RET
