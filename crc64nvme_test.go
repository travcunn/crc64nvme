// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

package crc64nvme

import (
	"bytes"
	"encoding"
	"hash"
	"hash/crc64"
	"math/rand/v2"
	"sync"
	"testing"
)

var oracleTable = crc64.MakeTable(Polynomial)

func oracle(crc uint64, p []byte) uint64 { return crc64.Update(crc, oracleTable, p) }

func TestCheckValue(t *testing.T) {
	if got := Checksum([]byte("123456789")); got != 0xae8b14860a799888 {
		t.Fatalf("got %#x want 0xae8b14860a799888", got)
	}
}

func TestUpdateEmpty(t *testing.T) {
	for _, crc := range []uint64{0, 1, 0xdeadbeefcafef00d, ^uint64(0)} {
		if got := Update(crc, nil); got != crc {
			t.Errorf("Update(%#x, nil) = %#x", crc, got)
		}
		if got := Update(crc, []byte{}); got != crc {
			t.Errorf("Update(%#x, empty) = %#x", crc, got)
		}
	}
}

func TestGenericMatchesOracle(t *testing.T) {
	rng := rand.NewChaCha8([32]byte{1})
	buf := make([]byte, 4096+16)
	rng.Read(buf)
	for size := 0; size <= 4096; size++ {
		for off := 0; off < 16; off++ {
			p := buf[off : off+size]
			crc0 := rng.Uint64()
			if got, want := updateGeneric(crc0, p), oracle(crc0, p); got != want {
				t.Fatalf("size %d off %d: got %#x want %#x", size, off, got, want)
			}
		}
	}
}

func TestHashStreaming(t *testing.T) {
	data := make([]byte, 100000)
	rand.NewChaCha8([32]byte{2}).Read(data)
	want := oracle(0, data)
	for _, chunk := range []int{1, 7, 15, 16, 17, 127, 128, 129, 1000, 4096} {
		h := New()
		for i := 0; i < len(data); i += chunk {
			end := i + chunk
			if end > len(data) {
				end = len(data)
			}
			n, err := h.Write(data[i:end])
			if err != nil || n != end-i {
				t.Fatal("Write")
			}
		}
		if got := h.Sum64(); got != want {
			t.Errorf("chunk %d: got %#x want %#x", chunk, got, want)
		}
		sum := h.Sum(nil)
		if len(sum) != Size || !bytes.Equal(sum, []byte{byte(want >> 56), byte(want >> 48), byte(want >> 40), byte(want >> 32), byte(want >> 24), byte(want >> 16), byte(want >> 8), byte(want)}) {
			t.Errorf("Sum bytes wrong: %x", sum)
		}
		if h.Sum64() != want {
			t.Error("Sum mutated state")
		}
		h.Reset()
		if h.Sum64() != 0 {
			t.Error("Reset")
		}
	}
}

func TestMarshal(t *testing.T) {
	h := New().(interface {
		Write([]byte) (int, error)
		Sum64() uint64
		MarshalBinary() ([]byte, error)
		UnmarshalBinary([]byte) error
	})
	h.Write([]byte("hello "))
	state, err := h.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	h2 := New().(interface {
		Write([]byte) (int, error)
		Sum64() uint64
		UnmarshalBinary([]byte) error
	})
	if err := h2.UnmarshalBinary(state); err != nil {
		t.Fatal(err)
	}
	h.Write([]byte("world"))
	h2.Write([]byte("world"))
	if h.Sum64() != h2.Sum64() || h.Sum64() != Checksum([]byte("hello world")) {
		t.Fatal("continuation after unmarshal differs")
	}
	for _, bad := range [][]byte{nil, []byte("crc\x02"), append([]byte("crc\x02"), make([]byte, 16)...), state[:len(state)-1], append(append([]byte{}, state...), 0)} {
		if err := h2.UnmarshalBinary(bad); err == nil {
			t.Errorf("accepted bad state %q", bad)
		}
	}
}

func TestSumAppends(t *testing.T) {
	h := New()
	h.Write([]byte("123456789"))
	prefix := []byte("prefix")
	in := append(make([]byte, 0, 64), prefix...)
	got := h.Sum(in)
	want := []byte("prefix\xae\x8b\x14\x86\x0a\x79\x98\x88")
	if !bytes.Equal(got, want) {
		t.Fatalf("Sum(%q) = %x, want %x", prefix, got, want)
	}
	if &got[0] != &in[0] {
		t.Error("Sum reallocated although the input had spare capacity")
	}
	if !bytes.Equal(in[:len(prefix)], prefix) {
		t.Errorf("Sum modified the prefix: %q", in[:len(prefix)])
	}
}

func TestAppendBinary(t *testing.T) {
	h := New()
	h.Write([]byte("hello "))
	ba, ok := h.(encoding.BinaryAppender)
	if !ok {
		t.Fatal("digest does not implement encoding.BinaryAppender")
	}
	prefix := []byte("prefix")
	state, err := ba.AppendBinary(append([]byte{}, prefix...))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(state, prefix) {
		t.Fatalf("AppendBinary lost the prefix: %q", state)
	}
	marshaled, err := h.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(state[len(prefix):], marshaled) {
		t.Fatalf("AppendBinary appended %x, MarshalBinary returned %x", state[len(prefix):], marshaled)
	}
	h2 := New()
	if err := h2.(encoding.BinaryUnmarshaler).UnmarshalBinary(state[len(prefix):]); err != nil {
		t.Fatal(err)
	}
	for _, d := range []hash.Hash64{h, h2} {
		d.Write([]byte("world"))
	}
	if h2.Sum64() != h.Sum64() || h2.Sum64() != Checksum([]byte("hello world")) {
		t.Fatalf("continuation after AppendBinary round trip: got %#x want %#x", h2.Sum64(), h.Sum64())
	}
}

func TestConcurrent(t *testing.T) {
	data := make([]byte, 1<<16)
	want := Checksum(data)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if Checksum(data) != want {
					t.Error("mismatch")
				}
			}
		}()
	}
	wg.Wait()
}
