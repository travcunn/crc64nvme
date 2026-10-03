// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

// Package bench compares this package against minio/crc64nvme and hash/crc64.
// It is a separate module so the library itself carries no benchmark-only
// dependencies. Sub-benchmarks are named size=N/impl=X so that
// "benchstat -col /impl" prints the implementations side by side.
package bench

import (
	"fmt"
	"hash/crc64"
	"testing"

	minio "github.com/minio/crc64nvme"
	ours "github.com/travcunn/crc64nvme"
)

var stdTable = crc64.MakeTable(ours.Polynomial)

var sizes = []int{64, 256, 1024, 4096, 65536, 1 << 20, 64 << 20}

func BenchmarkCompare(b *testing.B) {
	impls := []struct {
		name string
		sum  func([]byte) uint64
	}{
		{"travcunn", ours.Checksum},
		{"minio", minio.Checksum},
		{"stdlib", func(p []byte) uint64 { return crc64.Checksum(p, stdTable) }},
	}
	for _, size := range sizes {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i)
		}
		for _, impl := range impls {
			b.Run(fmt.Sprintf("size=%d/impl=%s", size, impl.name), func(b *testing.B) {
				b.SetBytes(int64(size))
				for b.Loop() {
					impl.sum(data)
				}
			})
		}
	}
}

func TestAgree(t *testing.T) {
	data := make([]byte, 100003)
	for i := range data {
		data[i] = byte(i * 31)
	}
	for _, size := range []int{0, 1, 15, 16, 17, 64, 255, 256, 257, 1024, 4096, 65536, len(data)} {
		p := data[:size]
		want := crc64.Checksum(p, stdTable)
		if got := ours.Checksum(p); got != want {
			t.Errorf("size %d: travcunn %#x, stdlib %#x", size, got, want)
		}
		if got := minio.Checksum(p); got != want {
			t.Errorf("size %d: minio %#x, stdlib %#x", size, got, want)
		}
	}
}
