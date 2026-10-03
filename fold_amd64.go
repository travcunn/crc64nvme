// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

package crc64nvme

// Kernels in fold_*_amd64.s. Contract: len(p) % 16 == 0 and len(p) >= 16.
func foldSSE(crc uint64, p []byte) uint64
func foldAVX2(crc uint64, p []byte) uint64
func foldAVX512(crc uint64, p []byte) uint64
