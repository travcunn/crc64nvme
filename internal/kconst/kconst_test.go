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

func TestBarrettConstants(t *testing.T) {
	// POLY = bitrev65(x^64 + P') mod 2^64: bit 0 is the x^64 term, bit j is P' bit 64-j.
	wantPoly := uint64(1)
	for j := 1; j < 64; j++ {
		wantPoly |= (polyNormal >> uint(64-j) & 1) << uint(j)
	}
	if POLY != wantPoly {
		t.Errorf("POLY got %#x want %#x", POLY, wantPoly)
	}
	// mu = x^128 div P. Long division of x^128 by the 65-bit P, bit serial.
	// Divide: start with dividend x^128. Quotient degree is 64. At each step k from 64
	// down to 0, if the current remainder has the x^(64+k) term, subtract P<<k.
	// Equivalent iterative form on 64-bit words:
	var rem uint64
	top := uint64(1) // the x^128 coefficient enters first
	var q uint64
	for k := 64; k >= 0; k-- {
		// rem holds coefficients x^(64+k-1 .. k) of the running dividend; top is x^(64+k).
		if top == 1 {
			rem ^= polyNormal
			if k < 64 {
				q |= 1 << uint(k)
			}
		}
		// shift in the next dividend coefficient (always zero after x^128)
		top = rem >> 63
		rem <<= 1
	}
	// q now holds quotient bits 63..0 (bit 64 is implicitly 1). MU = bitrev65(mu) mod 2^64.
	wantMu := uint64(1) // bit 0 <- quotient bit 64
	for j := 1; j < 64; j++ {
		wantMu |= (q >> uint(64-j) & 1) << uint(j)
	}
	if MU != wantMu {
		t.Errorf("MU got %#x want %#x", MU, wantMu)
	}
}
