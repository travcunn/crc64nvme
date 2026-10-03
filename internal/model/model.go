// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

// Package model is a bit-serial software model of the SIMD folding kernels. It
// exists to prove the constants and the reduction recipe against hash/crc64
// before any assembly is written, and to localise bugs afterwards: a kernel that
// disagrees with the oracle but agrees with the model has a math problem, one
// that disagrees with both has an assembly problem.
//
// The model does not mirror every kernel's structure. The arm64 pmull kernel
// folds 16 lanes down to 8 before its 8-lane loop, and the model has no
// counterpart to that step. The model proves the algebra instead (any lane
// count gives the same CRC), and TestKernelsMatchModel pins each kernel to it.
package model

import (
	"encoding/binary"

	"github.com/travcunn/crc64nvme/internal/kconst"
)

// lane is a 128-bit SIMD lane: lo holds bytes 0..7 of a 16-byte chunk, hi bytes 8..15.
type lane struct{ lo, hi uint64 }

func load(p []byte) lane {
	return lane{binary.LittleEndian.Uint64(p), binary.LittleEndian.Uint64(p[8:])}
}

func (a lane) xor(b lane) lane { return lane{a.lo ^ b.lo, a.hi ^ b.hi} }

// clmul is the 64x64 -> 128 carryless multiply (PCLMULQDQ / PMULL semantics).
func clmul(a, b uint64) lane {
	var r lane
	for i := 0; i < 64; i++ {
		if b>>uint(i)&1 == 1 {
			r.lo ^= a << uint(i)
			if i > 0 {
				r.hi ^= a >> uint(64-i)
			}
		}
	}
	return r
}

// fold multiplies the lane's polynomial by x^(8*dist) mod P.
func fold(a lane, dist int) lane {
	k := kconst.Pairs[dist/16-1]
	return clmul(a.lo, k[0]).xor(clmul(a.hi, k[1]))
}

// reduce turns one lane into the (reflected) CRC, spec section 5.4.
func reduce(r lane) uint64 {
	a := clmul(r.lo, kconst.K127)
	tLo, tHi := a.lo^r.hi, a.hi
	t2 := clmul(tLo, kconst.MU).lo
	d := clmul(t2, kconst.POLY)
	return tHi ^ d.hi ^ t2
}

// Fold continues crc over p exactly as a kernel with the given number of 16-byte
// accumulator lanes does. len(p) must be a non-zero multiple of 16. lanes must
// be between 1 and 32, the range for which kconst.Pairs holds a constant for
// the block distance 16*lanes.
func Fold(crc uint64, p []byte, lanes int) uint64 {
	if len(p) == 0 || len(p)%16 != 0 {
		panic("model.Fold: len(p) must be a non-zero multiple of 16")
	}
	c := ^crc
	block := 16 * lanes
	var acc lane
	if len(p) >= block {
		accs := make([]lane, lanes)
		for i := range accs {
			accs[i] = load(p[16*i:])
		}
		accs[0].lo ^= c
		p = p[block:]
		for len(p) >= block {
			for i := range accs {
				accs[i] = fold(accs[i], block).xor(load(p[16*i:]))
			}
			p = p[block:]
		}
		// Lane i is (lanes-1-i)*16 bytes ahead of the last lane.
		acc = accs[lanes-1]
		for i := 0; i < lanes-1; i++ {
			acc = acc.xor(fold(accs[i], 16*(lanes-1-i)))
		}
	} else {
		acc = load(p)
		acc.lo ^= c
		p = p[16:]
	}
	for len(p) >= 16 {
		acc = fold(acc, 16).xor(load(p))
		p = p[16:]
	}
	return ^reduce(acc)
}
