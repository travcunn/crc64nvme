// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

package model

import (
	"hash/crc64"
	"math/rand/v2"
	"testing"
)

var table = crc64.MakeTable(0x9a6c9329ac4bc9b5)

func TestFoldMatchesOracle(t *testing.T) {
	rng := rand.NewChaCha8([32]byte{3})
	buf := make([]byte, 4096)
	rng.Read(buf)
	for _, lanes := range []int{1, 4, 8, 16, 32} {
		for size := 16; size <= 4096; size += 16 {
			p := buf[:size]
			crc0 := rng.Uint64()
			want := crc64.Update(crc0, table, p)
			if got := Fold(crc0, p, lanes); got != want {
				t.Fatalf("lanes %d size %d: got %#x want %#x", lanes, size, got, want)
			}
		}
	}
}

// TestFoldSingleZeroBlock checks one 16-byte block of zeros with crc 0. It is
// shorter than any block, so no fold runs and the result depends only on the
// initial inversion and the Barrett reduction.
func TestFoldSingleZeroBlock(t *testing.T) {
	p := make([]byte, 16)
	if got, want := Fold(0, p, 8), crc64.Update(0, table, p); got != want {
		t.Fatalf("got %#x want %#x", got, want)
	}
}

func TestFoldPanicsOnBadLength(t *testing.T) {
	for _, n := range []int{0, 1, 17} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Fold with len(p) = %d did not panic", n)
				}
			}()
			Fold(0, make([]byte, n), 8)
		}()
	}
}
