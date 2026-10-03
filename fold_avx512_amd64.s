// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

#include "textflag.h"

// Fold all four 128-bit lanes of acc by the pair broadcast in Z4 and XOR in
// 64 bytes at off(SI). VPTERNLOGQ $0x96 is the three-way XOR, so the data
// merges with the low product in one step and the chain stays short.
#define FOLD_LANE512(acc, off) \
	VPCLMULQDQ $0x00, Z4, acc, Z5   \
	VPCLMULQDQ $0x11, Z4, acc, acc  \
	VMOVDQU64  off(SI), Z6          \
	VPTERNLOGQ $0x96, Z6, Z5, acc

// Fold all lanes of acc by the pair at koff(DX) and XOR the result into Z3.
#define COMBINE_LANE512(acc, koff) \
	VBROADCASTI32X4 koff(DX), Z4    \
	VPCLMULQDQ $0x00, Z4, acc, Z5   \
	VPCLMULQDQ $0x11, Z4, acc, acc  \
	VPTERNLOGQ $0x96, acc, Z5, Z3

// Fold acc by the pair at koff(DX) and XOR the result into X7.
#define COMBINE_LANE128(acc, koff) \
	VMOVDQU    koff(DX), X8         \
	VPCLMULQDQ $0x00, X8, acc, X9   \
	VPCLMULQDQ $0x11, X8, acc, acc  \
	VPXOR      X9, X7, X7           \
	VPXOR      acc, X7, X7

// Fold acc by the pair in X8 and XOR in 16 bytes at off(SI).
#define FOLD_LANE128(acc, off) \
	VPCLMULQDQ $0x00, X8, acc, X9   \
	VPCLMULQDQ $0x11, X8, acc, acc  \
	VPXOR      off(SI), X9, X9      \
	VPXOR      X9, acc, acc

// func foldAVX512(crc uint64, p []byte) uint64
TEXT ·foldAVX512(SB), NOSPLIT, $0-40
	MOVQ  crc+0(FP), AX
	MOVQ  p_base+8(FP), SI
	MOVQ  p_len+16(FP), CX
	LEAQ  ·foldK(SB), DX
	NOTQ  AX
	VMOVQ AX, X9                  // Z9 = ^crc in the low qword, zero above
	CMPQ  CX, $256
	JB    single

	VMOVDQU64 0(SI), Z0
	VMOVDQU64 64(SI), Z1
	VMOVDQU64 128(SI), Z2
	VMOVDQU64 192(SI), Z3
	VPXORQ    Z9, Z0, Z0
	ADDQ      $256, SI
	SUBQ      $256, CX
	VBROADCASTI32X4 240(DX), Z4   // d=256: K(2111), K(2047)
	CMPQ      CX, $256
	JB        combine

loop:
	FOLD_LANE512(Z0, 0)
	FOLD_LANE512(Z1, 64)
	FOLD_LANE512(Z2, 128)
	FOLD_LANE512(Z3, 192)
	ADDQ $256, SI
	SUBQ $256, CX
	CMPQ CX, $256
	JAE  loop

combine:
	// Zk is 64*(3-k) bytes ahead of Z3, lane for lane.
	COMBINE_LANE512(Z0, 176)      // d=192
	COMBINE_LANE512(Z1, 112)      // d=128
	COMBINE_LANE512(Z2, 48)       // d=64
	// Lane j of Z3 is 16*(3-j) bytes ahead of lane 3.
	VEXTRACTI32X4 $1, Z3, X1
	VEXTRACTI32X4 $2, Z3, X2
	VEXTRACTI32X4 $3, Z3, X7
	COMBINE_LANE128(X3, 32)       // d=48
	COMBINE_LANE128(X1, 16)       // d=32
	COMBINE_LANE128(X2, 0)        // d=16
	VMOVDQU X7, X0
	JMP     tail

single:
	VMOVDQU 0(SI), X0
	VPXOR   X9, X0, X0
	ADDQ    $16, SI
	SUBQ    $16, CX

tail:
	TESTQ   CX, CX
	JZ      reduce
	VMOVDQU 0(DX), X8             // d=16: K(191), K(127)

tailloop:
	FOLD_LANE128(X0, 0)
	ADDQ $16, SI
	SUBQ $16, CX
	JNZ  tailloop

reduce:
	LEAQ       ·reduceK(SB), DX
	VMOVDQU    0(DX), X8          // K127 in the low qword
	VPCLMULQDQ $0x00, X8, X0, X1  // X1 = A = clmul(R.lo, K127)
	VPSRLDQ    $8, X0, X0         // X0.lo = R.hi
	VPXOR      X0, X1, X1         // X1 = T: lo = A.lo ^ R.hi, hi = A.hi
	VMOVDQU    16(DX), X8         // MU
	VPCLMULQDQ $0x00, X8, X1, X2  // X2.lo = t2
	VMOVDQU    32(DX), X8         // POLY
	VPCLMULQDQ $0x00, X8, X2, X3  // X3 = D = clmul(t2, POLY)
	VPEXTRQ    $1, X1, AX         // T.hi
	VPEXTRQ    $1, X3, BX         // D.hi
	XORQ       BX, AX
	VMOVQ      X2, BX             // t2
	XORQ       BX, AX
	NOTQ       AX
	MOVQ       AX, ret+32(FP)
	VZEROUPPER
	RET
