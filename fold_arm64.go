// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build !purego

package crc64nvme

// Kernels in fold_*_arm64.s. Contract: len(p) % 16 == 0 and len(p) >= 16.
func foldPMULL(crc uint64, p []byte) uint64
func foldEOR3(crc uint64, p []byte) uint64
