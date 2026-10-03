// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

// Package crc64nvme implements the CRC-64/NVME checksum (the 64-bit CRC used by
// the NVM Express specification and by Amazon S3 CRC64NVME checksums) with
// carryless-multiply SIMD kernels for amd64 and arm64 and a pure-Go fallback.
package crc64nvme

import (
	"encoding/binary"
	"errors"
	"hash"
)

// Size is the size of a CRC-64 checksum in bytes.
const Size = 8

// Polynomial is the CRC-64/NVME polynomial in reversed (reflected) form, the
// convention used by hash/crc64. Normal form: 0xad93d23594c93659.
const Polynomial uint64 = 0x9a6c9329ac4bc9b5

// Checksum returns the CRC-64/NVME checksum of data.
func Checksum(data []byte) uint64 { return update(0, data) }

// Update returns the result of adding the bytes in p to the crc.
func Update(crc uint64, p []byte) uint64 { return update(crc, p) }

type digest struct{ crc uint64 }

// New returns a hash.Hash64 computing CRC-64/NVME. Sum lays the value out in
// big-endian order. The returned hash implements encoding.BinaryMarshaler,
// encoding.BinaryAppender and encoding.BinaryUnmarshaler.
func New() hash.Hash64 { return &digest{} }

func (d *digest) Size() int      { return Size }
func (d *digest) BlockSize() int { return 1 }
func (d *digest) Reset()         { d.crc = 0 }
func (d *digest) Sum64() uint64  { return d.crc }

func (d *digest) Write(p []byte) (int, error) {
	d.crc = update(d.crc, p)
	return len(p), nil
}

func (d *digest) Sum(in []byte) []byte {
	return binary.BigEndian.AppendUint64(in, d.crc)
}

const (
	marshalMagic = "crc64nvme\x01"
	marshalSize  = len(marshalMagic) + Size
)

func (d *digest) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, marshalMagic...)
	return binary.BigEndian.AppendUint64(b, d.crc), nil
}

func (d *digest) MarshalBinary() ([]byte, error) {
	return d.AppendBinary(make([]byte, 0, marshalSize))
}

var (
	errMarshalMagic = errors.New("crc64nvme: invalid hash state identifier")
	errMarshalSize  = errors.New("crc64nvme: invalid hash state size")
)

func (d *digest) UnmarshalBinary(b []byte) error {
	if len(b) < len(marshalMagic) || string(b[:len(marshalMagic)]) != marshalMagic {
		return errMarshalMagic
	}
	if len(b) != marshalSize {
		return errMarshalSize
	}
	d.crc = binary.BigEndian.Uint64(b[len(marshalMagic):])
	return nil
}
