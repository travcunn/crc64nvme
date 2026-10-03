// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

package kconst

import "testing"

// Independent bit-serial GF(2)[x] arithmetic, deliberately written without
// looking at internal/gen.
const polyNormal uint64 = 0xad93d23594c93659 // P without the x^64 term

// mulmod returns a*b mod P for a, b of degree < 64.
func mulmod(a, b uint64) uint64 {
	var r uint64
	for i := 63; i >= 0; i-- {
		carry := r >> 63
		r <<= 1
		if carry == 1 {
			r ^= polyNormal
		}
		if b>>uint(i)&1 == 1 {
			r ^= a
		}
	}
	return r
}

// xpow returns x^e mod P.
func xpow(e int) uint64 {
	r, b := uint64(1), uint64(2)
	for e > 0 {
		if e&1 == 1 {
			r = mulmod(r, b)
		}
		b = mulmod(b, b)
		e >>= 1
	}
	return r
}

func rev64(v uint64) uint64 {
	var r uint64
	for i := 0; i < 64; i++ {
		r = r<<1 | v&1
		v >>= 1
	}
	return r
}

func TestFoldConstants(t *testing.T) {
	for i, pair := range Pairs {
		d := 16 * (i + 1)
		if want := rev64(xpow(8*d + 63)); pair[0] != want {
			t.Errorf("d=%d lo: got %#x want %#x", d, pair[0], want)
		}
		if want := rev64(xpow(8*d - 1)); pair[1] != want {
			t.Errorf("d=%d hi: got %#x want %#x", d, pair[1], want)
		}
		if v, ok := K(8*d + 63); !ok || v != pair[0] {
			t.Errorf("K(%d) lookup", 8*d+63)
		}
	}
	if K127 != rev64(xpow(127)) || K127 != Pairs[0][1] {
		t.Error("K127")
	}
}

// clmul returns the 128-bit bit-serial carryless product of a and b.
func clmul(a, b uint64) (lo, hi uint64) {
	for i := uint(0); i < 64; i++ {
		if b>>i&1 == 1 {
			lo ^= a << i
			if i > 0 {
				hi ^= a >> (64 - i)
			}
		}
	}
	return lo, hi
}

func TestBarrettConstants(t *testing.T) {
	// Literal values fixed by the spec.
	if MU != 0x27ecfa329aef9f77 {
		t.Errorf("MU got %#x want 0x27ecfa329aef9f77", MU)
	}
	if POLY != 0x34d926535897936b {
		t.Errorf("POLY got %#x want 0x34d926535897936b", POLY)
	}
	if K127 != 0x21e9761e252621ac {
		t.Errorf("K127 got %#x want 0x21e9761e252621ac", K127)
	}

	// POLY = bitrev65(x^64 + P') mod 2^64: bit 0 is the x^64 term, bit j is P' bit 64-j.
	wantPoly := uint64(1)
	for j := 1; j < 64; j++ {
		wantPoly |= (polyNormal >> uint(64-j) & 1) << uint(j)
	}
	if POLY != wantPoly {
		t.Errorf("POLY got %#x want %#x", POLY, wantPoly)
	}

	// MU = bitrev65(mu) mod 2^64 where mu = x^128 div P has degree 64. Bit 0 of MU
	// is the implicit x^64 quotient term. Un-reflect the rest to get q, the low 64
	// quotient bits, then check the division identity
	//   x^128 = (x^64 + q)(x^64 + P') + r,  deg r < 64.
	// Expanding, x^128 cancels and the x^64..x^127 coefficients of the right side must
	// vanish, so the high word of clmul(q, P') must equal q XOR P'.
	if MU&1 != 1 {
		t.Errorf("MU bit 0 = 0, want the implicit x^64 quotient term")
	}
	var q uint64
	for j := 1; j < 64; j++ {
		q |= (MU >> uint(j) & 1) << uint(64-j)
	}
	if _, hi := clmul(q, polyNormal); hi != q^polyNormal {
		t.Errorf("x^128 != (x^64+q)(x^64+P') + r: clmul high word %#x, want q^P' = %#x", hi, q^polyNormal)
	}
}
