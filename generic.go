// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

package crc64nvme

import (
	"encoding/binary"
	"sync"
)

// byteTable is the classic 256-entry table for the reflected polynomial.
var byteTable = makeByteTable()

func makeByteTable() *[256]uint64 {
	var t [256]uint64
	for i := range t {
		crc := uint64(i)
		for j := 0; j < 8; j++ {
			if crc&1 == 1 {
				crc = crc>>1 ^ Polynomial
			} else {
				crc >>= 1
			}
		}
		t[i] = crc
	}
	return &t
}

var (
	slicingOnce  sync.Once
	slicingTable *[8][256]uint64
)

// slicing returns the slicing-by-8 table, built on first use (16 KiB).
func slicing() *[8][256]uint64 {
	slicingOnce.Do(func() {
		var t [8][256]uint64
		t[0] = *byteTable
		for i := 0; i < 256; i++ {
			crc := t[0][i]
			for k := 1; k < 8; k++ {
				crc = t[0][crc&0xff] ^ crc>>8
				t[k][i] = crc
			}
		}
		slicingTable = &t
	})
	return slicingTable
}

// updateGeneric continues crc over p without SIMD. It is the only path under
// the purego build tag and finishes the tail every kernel leaves behind.
func updateGeneric(crc uint64, p []byte) uint64 {
	crc = ^crc
	if len(p) >= 64 {
		t := slicing()
		for len(p) >= 8 {
			crc ^= binary.LittleEndian.Uint64(p)
			crc = t[7][crc&0xff] ^
				t[6][crc>>8&0xff] ^
				t[5][crc>>16&0xff] ^
				t[4][crc>>24&0xff] ^
				t[3][crc>>32&0xff] ^
				t[2][crc>>40&0xff] ^
				t[1][crc>>48&0xff] ^
				t[0][crc>>56]
			p = p[8:]
		}
	}
	for _, b := range p {
		crc = byteTable[byte(crc)^b] ^ crc>>8
	}
	return ^crc
}
