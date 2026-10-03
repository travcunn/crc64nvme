# crc64nvme

[![CI](https://github.com/travcunn/crc64nvme/actions/workflows/ci.yml/badge.svg)](https://github.com/travcunn/crc64nvme/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/travcunn/crc64nvme.svg)](https://pkg.go.dev/github.com/travcunn/crc64nvme)

Package crc64nvme computes CRC-64/NVME, the checksum defined by the NVMe specification and used by Amazon S3 as CRC64NVME.
It has the same API shape as `hash/crc64`, with carry-less-multiply kernels for amd64 and arm64 selected at init and a pure-Go fallback for everything else.
At 1 MiB it measures 107.5 GB/s on an Apple M4 and 55.1 GB/s on an AMD EPYC 9654P, single core.

It is an independent implementation written from the public specification and the Intel folding paper, licensed Apache 2.0, with no dependency other than `golang.org/x/sys/cpu`.

## Install

```sh
go get github.com/travcunn/crc64nvme
```

Requires Go 1.25 or newer.

## Usage

Checksum a byte slice in one call:

```go
sum := crc64nvme.Checksum([]byte("123456789")) // 0xae8b14860a799888, the CRC-64/NVME check value
```

Stream a reader through the `hash.Hash64` returned by `New`:

```go
h := crc64nvme.New()
if _, err := io.Copy(h, f); err != nil {
	return err
}
sum := h.Sum64()  // as a uint64
raw := h.Sum(nil) // as 8 big-endian bytes
```

Continue a checksum across separate buffers with `Update`:

```go
crc := crc64nvme.Checksum(part1)
crc = crc64nvme.Update(crc, part2) // equals Checksum of part1 followed by part2
```

The digest returned by `New` implements `encoding.BinaryMarshaler`, `encoding.BinaryAppender` and `encoding.BinaryUnmarshaler`, so a partial checksum can be saved and resumed.

## Performance

All numbers are single-core throughput in GB/s (10^9 bytes per second, higher is better), measured on 2026-10-03 with Go 1.26.4 using `go test -bench` and the benchstat median.
The two machines are an Apple M4 (macOS 26.2, performance core, measured clock 4.35 GHz) and an AMD EPYC 9654P (Zen 4, Linux, 32-vCPU VM, measured clock 3.64 GHz).
Both machines were shared with other work during the runs.
Tables A and B are the median of 10 runs with the 1-minute load average at 2.4 at the start and 2.5 at the end on the M4, and 9.3 and 4.6 on the EPYC.
Tables C and D are the median of 5 runs with load averages of 2.4 to 2.5 on the M4 and 3.2 to 4.1 on the EPYC.
All implementations in a table ran in the same `go test` invocation.

**Table A. Apple M4 (macOS, arm64)**

| Size   | this package (pmull) | hash/crc64 |
|-------:|---------------------:|-----------:|
| 64 B   |                 10.6 |        0.8 |
| 256 B  |                 35.4 |        0.6 |
| 1 KiB  |                 71.6 |        0.5 |
| 4 KiB  |                 95.6 |        1.5 |
| 64 KiB |                111.4 |        2.1 |
| 1 MiB  |                107.5 |        2.2 |
| 64 MiB |                 67.5 |        2.2 |

**Table B. AMD EPYC 9654P (Linux, amd64, Zen 4)**

| Size   | this package (avx512) | hash/crc64 |
|-------:|----------------------:|-----------:|
| 64 B   |                   5.4 |        0.5 |
| 256 B  |                  16.6 |        0.5 |
| 1 KiB  |                  35.7 |        0.5 |
| 4 KiB  |                  50.3 |        1.1 |
| 64 KiB |                  56.9 |        1.8 |
| 1 MiB  |                  55.1 |        1.8 |
| 64 MiB |                  23.6 |        1.9 |

On the EPYC the 64 B row runs the avx2 kernel (see [How a kernel is chosen](#how-a-kernel-is-chosen)).

**Table C. Per-kernel throughput**

Each kernel is forced in turn with `BenchmarkTiers` in the root package.

| Machine | Kernel  | 4 KiB | 1 MiB |
|---------|---------|------:|------:|
| M4      | generic |   2.2 |   2.2 |
| M4      | eor3    |  69.6 |  81.2 |
| M4      | pmull   |  95.8 | 110.3 |
| EPYC    | generic |   1.9 |   1.9 |
| EPYC    | sse     |  13.8 |  13.8 |
| EPYC    | avx2    |  26.7 |  27.8 |
| EPYC    | avx512  |  50.1 |  54.6 |

**Table D. How close to the hardware limit**

The bound is the carry-less-multiply issue rate of the main loop, at the measured clock.
Each 16-byte lane costs two 64x64 multiplies per block.

| Kernel | Bound (B/cycle) | Bound (GB/s) | Measured 1 MiB (GB/s) | Of bound |
|--------|----------------:|-------------:|----------------------:|---------:|
| sse    |             4.0 |         14.6 |                  13.8 |      94% |
| avx2   |             8.0 |         29.1 |                  27.8 |      95% |
| avx512 |            16.0 |         58.2 |                  54.6 |      94% |
| pmull  |            32.0 |        139.2 |                 110.3 |      79% |
| eor3   |            21.3 |         92.8 |                  81.2 |      88% |

Zen 4 issues PCLMULQDQ at one per 2 cycles with 4-cycle latency for the xmm, ymm and zmm forms alike [6].
A zmm multiply covers four lanes where a ymm multiply covers two, so the AVX-512 bound is twice the AVX2 bound.
Apple cores issue PMULL and PMULL2 on four SIMD pipes with 3-cycle latency, and EOR3 on the same pipes [8].
They also fuse a PMULL with an EOR into the same register into one micro-op [8].
The pmull kernel's main loop pairs every multiply that way, which makes a lane cost 2 micro-ops against 3 for the eor3 kernel (PMULL, PMULL2, EOR3).
On four pipes that is 16 micro-ops (4 cycles) per 128 bytes for pmull and 24 (6 cycles) for eor3.
The Apple bounds use Firestorm data, the closest published measurements for the M4.

At 64 MiB the input no longer fits in cache, and both machines are limited by memory bandwidth rather than by the kernels.

Reproduce the per-kernel numbers from the repository root:

```sh
go test -run xxx -bench Tiers -count 10 . | benchstat -
```

## How a kernel is chosen

At init the package reads the CPU features through `golang.org/x/sys/cpu` and selects one kernel.
Kernels process whole 16-byte chunks. The last 0 to 15 bytes, and inputs under 16 bytes, go through the generic code.

| Kernel  | Arch  | CPU features required                                 | Block per iteration | Accumulators | Notes |
|---------|-------|-------------------------------------------------------|--------------------:|-------------:|-------|
| generic | any   | none                                                   | 8 B                 | n/a          | Slicing-by-8 tables. The only path with `-tags purego` or on other architectures. |
| sse     | amd64 | SSE4.1, PCLMULQDQ                                     | 128 B               | 8 xmm        | |
| avx2    | amd64 | AVX2, PCLMULQDQ, VPCLMULQDQ                           | 256 B               | 8 ymm        | VPCLMULQDQ is read from CPUID directly so the VEX-256 form is found on CPUs without AVX-512, such as Zen 3. |
| avx512  | amd64 | AVX512F, AVX512VL, VPCLMULQDQ                         | 256 B               | 4 zmm        | Used for inputs of 256 bytes or more. Shorter inputs use avx2. |
| pmull   | arm64 | PMULL                                                  | 256 B               | 16           | Preferred on macOS because Apple cores fuse PMULL with EOR. Remainders of 128 to 255 bytes run an 8-lane loop. |
| eor3    | arm64 | PMULL, SHA3                                            | 128 B               | 8            | Preferred on other operating systems when SHA3 is present. |

Each zmm, ymm or xmm accumulator holds four, two or one 16-byte lanes, so every SIMD kernel keeps 8 or 16 lanes in flight.
`x/sys/cpu` does not expose the arm64 implementer, so the operating system stands in for "Apple core".
Asahi Linux on Apple silicon therefore gets eor3, which is correct and slower.

The 256-byte threshold for avx512 is the AVX-512 block size.

Building with `-tags purego` removes all assembly and uses the generic kernel everywhere.

## Design

**Folding.**
Each 16-byte lane of input is a 128-bit polynomial.
Moving a lane forward by `d` bytes multiplies it by x^(8d) mod P, and a fold constant is that multiplier split into the pair K(8d+63) and K(8d-1), where K(e) = bitrev64(x^e mod P).
The kernel multiplies the lane's low and high 64-bit halves by the pair, XORs the two 128-bit products, and XORs in the data that sits `d` bytes later.
The main loop does this once per lane per iteration, with `d` equal to the block size, so each iteration consumes one block.
The lanes are independent, so the multiplies for 8 or 16 lanes overlap in the pipeline.

```
  lane (16 B)                       fold constant pair for d bytes
  +-----------+-----------+         +-------------+-------------+
  |  lo  64b  |  hi  64b  |         |  K(8d+63)   |  K(8d-1)    |
  +-----+-----+-----+-----+         +------+------+------+------+
        |           |                      |             |
        +-------- clmul(lo, K(8d+63)) -----+             |
                    |                                    |
                    +------ clmul(hi, K(8d-1)) ----------+
                                     |
            128-bit product ^ 128-bit product ^ next 16 B of data at +d
                                     |
                                     v
                                 new lane
```

**Lane combination.**
After the last full block, lane `i` of `n` is `16*(n-1-i)` bytes ahead of the final lane.
Each earlier lane is folded by its own distance with the matching constant pair and XORed into the final lane, which leaves one 128-bit remainder.
Any whole 16-byte chunks left after the last block are folded into that remainder one at a time with the d = 16 pair.

**Barrett reduction.**
The 128-bit remainder is reduced to 64 bits with three carry-less multiplies: one by K(127) to fold the low half onto the high half, one by MU = x^128 div P to estimate the quotient, and one by P to subtract it.
The data is bit-reflected, so every value a step needs sits in the low or high 64-bit half of a product, and the reduction needs no shift instructions.
MU and P each have degree 64. The generator stores their 65-bit reflections truncated to 64 bits, and the dropped bit of P is restored by one extra XOR of the quotient estimate.

**Constants and proof.**
Every constant is derived from the polynomial by `internal/gen` and written to `internal/kconst` and `kconst_data.s`. None is typed by hand.
A bit-serial software model in `internal/model` performs the same folds and reduction with the same constants, and it is tested against `hash/crc64` for 8, 16 and 32 lanes before any assembly runs.
Every kernel is tested against the model at each 16-byte multiple up to 1024 bytes, and against `hash/crc64` at every size from 0 to 2048 bytes at 16 alignments, plus large and streaming cases.

## Correctness and testing

- The oracle is the standard library's `hash/crc64` with the CRC-64/NVME polynomial.
- Every size from 0 to 2048 bytes is checked at 16 buffer offsets with a random starting CRC, for every kernel the machine can run.
- Large inputs up to 64 MiB, including lengths that are not multiples of 16, are checked for every kernel.
- Streaming is checked with fixed chunk sizes around the 16, 128 and 256-byte boundaries and with 200 random split patterns.
- Each test forces every kernel available on the machine, and lowers the avx512 threshold so that kernel also sees short inputs.
- The fold and Barrett constants are recomputed in tests by independent code and compared with the generated tables.
- A CI job runs the arm64 tests under QEMU emulating a Cortex-A72, which has PMULL but no SHA3, to cover the pmull-only selection outside macOS.
- CI runs `go generate` and fails if the generated files differ from the committed ones.

## Sources

1. Intel, "Fast CRC Computation for Generic Polynomials Using PCLMULQDQ Instruction". <https://www.intel.com/content/dam/www/public/us/en/documents/white-papers/fast-crc-computation-generic-polynomials-pclmulqdq-paper.pdf>. Folding and the reduction recipe.
2. NVM Express, NVM Command Set Specification. <https://nvmexpress.org/specifications/>. Definition of the 64-bit CRC.
3. Go standard library, `hash/crc64`. <https://pkg.go.dev/hash/crc64>. Test oracle and API shape.
4. Greg Cook, Catalogue of parametrised CRC algorithms, CRC-64/NVME entry. <https://reveng.sourceforge.io/crc-catalogue/all.htm#crc.cat.crc-64-nvme>. Parameters and check value.
5. P. Barrett, "Implementing the Rivest Shamir and Adleman Public Key Encryption Algorithm on a Standard Digital Signal Processor", CRYPTO '86. <https://link.springer.com/chapter/10.1007/3-540-47721-7_24>. Barrett reduction.
6. uops.info, Zen 4 measurements for [PCLMULQDQ xmm](https://uops.info/html-instr/PCLMULQDQ_XMM_XMM_I8.html), [VPCLMULQDQ ymm](https://uops.info/html-instr/VPCLMULQDQ_YMM_YMM_YMM_I8.html) and [VPCLMULQDQ zmm](https://uops.info/html-instr/VPCLMULQDQ_ZMM_ZMM_ZMM_I8.html). <https://uops.info>. Latency and throughput for Table D.
7. Agner Fog, Instruction tables. <https://www.agner.org/optimize/instruction_tables.pdf>. Cross-check of x86 latencies.
8. Dougall Johnson, Apple Firestorm [SIMD instruction tables](https://dougallj.github.io/applecpu/firestorm-simd.html) and [microarchitecture notes, including instruction fusion](https://dougallj.github.io/applecpu/firestorm.html). PMULL and EOR3 throughput and PMULL+EOR fusion for Table D and the pmull kernel.
9. Arm, Neoverse N1, V1 and V2 Software Optimization Guides. <https://developer.arm.com/documentation>. Kernel choice on non-Apple arm64 cores.
10. `golang.org/x/sys/cpu`. <https://pkg.go.dev/golang.org/x/sys/cpu>. CPU feature detection.
11. A Quick Guide to Go's Assembler. <https://go.dev/doc/asm>. Assembly conventions.

## License

Apache License 2.0. See [LICENSE](LICENSE).
