// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

package model

import (
	"hash/crc64"
	"math/rand"
	"testing"
)

var table = crc64.MakeTable(0x9a6c9329ac4bc9b5)

func TestFoldMatchesOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	buf := make([]byte, 4096)
	rng.Read(buf)
	for _, lanes := range []int{8, 16, 32} {
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

func TestFoldCheckValue(t *testing.T) {
	// "123456789" padded is not the check value; instead confirm one 16-byte block
	// of zeros with crc 0 equals the oracle, which exercises init handling alone.
	p := make([]byte, 16)
	if got, want := Fold(0, p, 8), crc64.Update(0, table, p); got != want {
		t.Fatalf("got %#x want %#x", got, want)
	}
}
