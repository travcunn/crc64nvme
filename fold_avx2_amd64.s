// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

#include "textflag.h"

// Fold both 128-bit lanes of acc by the pair broadcast in Y8 and XOR in 32 bytes
// at off(SI). The data is merged into the clmul temp first so the load overlaps
// the second multiply.
#define FOLD_LANE256(acc, off) \
	VPCLMULQDQ $0x00, Y8, acc, Y9   \
	VPCLMULQDQ $0x11, Y8, acc, acc  \
	VPXOR      off(SI), Y9, Y9      \
	VPXOR      Y9, acc, acc

// Fold both lanes of acc by the pair at koff(DX) and XOR the result into Y7.
#define COMBINE_LANE256(acc, koff) \
	VBROADCASTI128 koff(DX), Y8     \
	VPCLMULQDQ $0x00, Y8, acc, Y9   \
	VPCLMULQDQ $0x11, Y8, acc, acc  \
	VPXOR      Y9, Y7, Y7           \
	VPXOR      acc, Y7, Y7

// Fold acc by the pair in X8 and XOR in 16 bytes at off(SI).
#define FOLD_LANE128(acc, off) \
	VPCLMULQDQ $0x00, X8, acc, X9   \
	VPCLMULQDQ $0x11, X8, acc, acc  \
	VPXOR      off(SI), X9, X9      \
	VPXOR      X9, acc, acc

// func foldAVX2(crc uint64, p []byte) uint64
TEXT ·foldAVX2(SB), NOSPLIT, $0-40
	MOVQ  crc+0(FP), AX
	MOVQ  p_base+8(FP), SI
	MOVQ  p_len+16(FP), CX
	LEAQ  ·foldK(SB), DX
	NOTQ  AX
	VMOVQ AX, X9                  // Y9 = ^crc in the low qword, zero above
	CMPQ  CX, $256
	JB    single

	VMOVDQU 0(SI), Y0
	VMOVDQU 32(SI), Y1
	VMOVDQU 64(SI), Y2
	VMOVDQU 96(SI), Y3
	VMOVDQU 128(SI), Y4
	VMOVDQU 160(SI), Y5
	VMOVDQU 192(SI), Y6
	VMOVDQU 224(SI), Y7
	VPXOR   Y9, Y0, Y0
	ADDQ    $256, SI
	SUBQ    $256, CX
	VBROADCASTI128 240(DX), Y8    // d=256: K(2111), K(2047)
	CMPQ    CX, $256
	JB      combine

loop:
	FOLD_LANE256(Y0, 0)
	FOLD_LANE256(Y1, 32)
	FOLD_LANE256(Y2, 64)
	FOLD_LANE256(Y3, 96)
	FOLD_LANE256(Y4, 128)
	FOLD_LANE256(Y5, 160)
	FOLD_LANE256(Y6, 192)
	FOLD_LANE256(Y7, 224)
	ADDQ $256, SI
	SUBQ $256, CX
	CMPQ CX, $256
	JAE  loop

combine:
	// Yk is 32*(7-k) bytes ahead of Y7, lane for lane: constant entry 2*(7-k)-1.
	COMBINE_LANE256(Y0, 208)      // d=224
	COMBINE_LANE256(Y1, 176)      // d=192
	COMBINE_LANE256(Y2, 144)      // d=160
	COMBINE_LANE256(Y3, 112)      // d=128
	COMBINE_LANE256(Y4, 80)       // d=96
	COMBINE_LANE256(Y5, 48)       // d=64
	COMBINE_LANE256(Y6, 16)       // d=32
	VEXTRACTI128 $1, Y7, X1       // upper lane, the later 16 bytes
	VMOVDQU    0(DX), X8          // d=16: K(191), K(127)
	VPCLMULQDQ $0x00, X8, X7, X9
	VPCLMULQDQ $0x11, X8, X7, X7
	VPXOR      X9, X1, X1
	VPXOR      X7, X1, X0
	JMP        tail

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
