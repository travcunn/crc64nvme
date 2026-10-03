// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

package crc64nvme

import (
	"fmt"
	"testing"
)

func BenchmarkTiers(b *testing.B) {
	oldTier, oldMin := bestTier, avx512Min
	b.Cleanup(func() { bestTier, avx512Min = oldTier, oldMin })
	for _, size := range []int{64, 1024, 4096, 65536, 1 << 20, 64 << 20} {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i)
		}
		for _, tr := range availableTiers {
			b.Run(fmt.Sprintf("%s/%d", tr, size), func(b *testing.B) {
				bestTier = tr
				avx512Min = 16
				b.SetBytes(int64(size))
				for i := 0; i < b.N; i++ {
					Checksum(data)
				}
			})
		}
	}
}
