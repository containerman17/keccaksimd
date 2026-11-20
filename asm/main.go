// Copyright (C) 2026, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Command asm writes ../keccak_amd64.s and ../stub_amd64.go: Keccak-f[1600] absorption of 4
// (AVX2) or 8 (AVX-512) independent messages, one 64-bit state lane of each message per
// vector element. The AVX2 kernel keeps the two state copies in memory (16 ymm registers);
// the AVX-512 kernel keeps all 25 lanes in Z0-Z24 and renames registers for pi, so its 24
// rounds are unrolled (pi has order 24, the mapping returns to identity).
//
// Run via go generate in the parent package. avo does not emit a license header, so main
// prepends it to the stub file after generation.
package main

import (
	"flag"
	"fmt"
	"os"

	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	. "github.com/mmcloughlin/avo/reg"
)

var rc = [24]uint64{
	0x0000000000000001, 0x0000000000008082, 0x800000000000808A, 0x8000000080008000,
	0x000000000000808B, 0x0000000080000001, 0x8000000080008081, 0x8000000000008009,
	0x000000000000008A, 0x0000000000000088, 0x0000000080008009, 0x000000008000000A,
	0x000000008000808B, 0x800000000000008B, 0x8000000000008089, 0x8000000000008003,
	0x8000000000008002, 0x8000000000000080, 0x000000000000800A, 0x800000008000000A,
	0x8000000080008081, 0x8000000000008080, 0x0000000080000001, 0x8000000080008008,
}

// rho[x+5y] is the rotation of lane (x, y).
var rho = [25]uint64{
	0, 1, 62, 28, 27,
	36, 44, 6, 55, 20,
	3, 10, 43, 25, 39,
	41, 45, 15, 21, 8,
	18, 2, 61, 56, 14,
}

const rateWords = 17 // 136-byte rate

var rcMem Mem

func main() {
	rcMem = GLOBL("rc", RODATA|NOPTR)
	for i, c := range rc {
		DATA(8*i, U64(c))
	}
	avx2()
	avx512()
	Generate()
	prependLicense(flag.Lookup("stubs").Value.String())
}

// tern returns the VPTERNLOGQ immediate for f(dest, b, c) (Go operand order: $imm, c, b, dest).
func tern(f func(a, b, c bool) bool) U8 {
	var imm uint8
	for i := 0; i < 8; i++ {
		if f(i&4 != 0, i&2 != 0, i&1 != 0) {
			imm |= 1 << i
		}
	}
	return U8(imm)
}

var (
	xor3 = tern(func(a, b, c bool) bool { return a != b != c })
	chi  = tern(func(a, b, c bool) bool { return a != (!b && c) })
)

// src returns the input lane that pi moves to output lane (X, Y): B[y][2x+3y] = A[x][y].
func src(X, Y int) int { return (3*(Y-3*X)%5+15)%5 + 5*X }

func avx2() {
	TEXT("absorb4", 0, "func(st *[25][4]uint64, p *[4]*byte, blocks int)")
	Pragma("noescape")
	Doc("absorb4 absorbs blocks 136-byte blocks of 4 messages (p[k] advances 136 per block) into st.")
	tmp := AllocLocal(25 * 32)
	st, p, n := RDI, RSI, RCX
	Load(Param("st"), st)
	Load(Param("p"), p)
	Load(Param("blocks"), n)
	TESTQ(n, n)
	JZ(LabelRef("done4"))
	idx := Y15
	VMOVDQU(Mem{Base: p}, idx)
	VPBROADCASTQ(Mem{Base: p}, Y14)
	VPSUBQ(Y14, idx, idx)
	base := R8
	MOVQ(Mem{Base: p}, base)
	Label("block4")
	for w := 0; w < rateWords; w++ {
		VPCMPEQQ(Y13, Y13, Y13)
		VPGATHERQQ(Y13, Mem{Base: base, Index: idx, Scale: 1, Disp: 8 * w}, Y14)
		VPXOR(Mem{Base: st, Disp: 32 * w}, Y14, Y14)
		VMOVDQU(Y14, Mem{Base: st, Disp: 32 * w})
	}
	rcp, cnt, t := R9, R10, R11
	LEAQ(rcMem, rcp)
	MOVQ(U32(12), cnt)
	LEAQ(tmp, t)
	Label("round4")
	round4(st, t, 0, rcp)
	round4(t, st, 8, rcp)
	ADDQ(U8(16), rcp)
	DECQ(cnt)
	JNZ(LabelRef("round4"))
	ADDQ(U32(136), base)
	DECQ(n)
	JNZ(LabelRef("block4"))
	Label("done4")
	VZEROUPPER()
	RET()
}

// round4 reads the state at a and writes the next state at e. Y0-Y4 hold C then B, Y5-Y9 D.
func round4(a, e GPPhysical, rcOff int, rcp GPPhysical) {
	lane := func(r GPPhysical, i int) Mem { return Mem{Base: r, Disp: 32 * i} }
	C := []VecPhysical{Y0, Y1, Y2, Y3, Y4}
	D := []VecPhysical{Y5, Y6, Y7, Y8, Y9}
	t, u := Y10, Y11
	for x := 0; x < 5; x++ {
		VMOVDQU(lane(a, x), C[x])
		for y := 1; y < 5; y++ {
			VPXOR(lane(a, x+5*y), C[x], C[x])
		}
	}
	for x := 0; x < 5; x++ {
		VPSLLQ(U8(1), C[(x+1)%5], t)
		VPSRLQ(U8(63), C[(x+1)%5], D[x])
		VPOR(t, D[x], D[x])
		VPXOR(C[(x+4)%5], D[x], D[x])
	}
	B := C
	for Y := 0; Y < 5; Y++ {
		for X := 0; X < 5; X++ {
			s := src(X, Y)
			VPXOR(lane(a, s), D[s%5], B[X])
			if r := rho[s]; r != 0 {
				VPSLLQ(U8(r), B[X], t)
				VPSRLQ(U8(64-r), B[X], B[X])
				VPOR(t, B[X], B[X])
			}
		}
		for X := 0; X < 5; X++ {
			VPANDN(B[(X+2)%5], B[(X+1)%5], t)
			VPXOR(B[X], t, t)
			if X == 0 && Y == 0 {
				VPBROADCASTQ(Mem{Base: rcp, Disp: rcOff}, u)
				VPXOR(u, t, t)
			}
			VMOVDQU(t, lane(e, X+5*Y))
		}
	}
}

func avx512() {
	TEXT("absorb8", NOSPLIT, "func(st *[25][8]uint64, p *[8]*byte, blocks int)")
	Pragma("noescape")
	Doc("absorb8 absorbs blocks 136-byte blocks of 8 messages (p[k] advances 136 per block) into st.")
	idxMem := AllocLocal(64)
	st, p, n := RDI, RSI, RCX
	Load(Param("st"), st)
	Load(Param("p"), p)
	Load(Param("blocks"), n)
	TESTQ(n, n)
	JZ(LabelRef("done8"))
	Z := []VecPhysical{Z0, Z1, Z2, Z3, Z4, Z5, Z6, Z7, Z8, Z9, Z10, Z11, Z12, Z13, Z14, Z15,
		Z16, Z17, Z18, Z19, Z20, Z21, Z22, Z23, Z24, Z25, Z26, Z27, Z28, Z29, Z30, Z31}
	VMOVDQU64(Mem{Base: p}, Z25)
	VPBROADCASTQ(Mem{Base: p}, Z26)
	VPSUBQ(Z26, Z25, Z25)
	VMOVDQU64(Z25, idxMem)
	base := R8
	MOVQ(Mem{Base: p}, base)
	for i := 0; i < 25; i++ {
		VMOVDQU64(Mem{Base: st, Disp: 64 * i}, Z[i])
	}
	Label("block8")
	VMOVDQU64(idxMem, Z26)
	for w := 0; w < rateWords; w++ {
		KXNORW(K1, K1, K1)
		VPGATHERQQ(Mem{Base: base, Index: Z26, Scale: 1, Disp: 8 * w}, K1, Z25)
		VPXORQ(Z25, Z[w], Z[w])
	}
	var ph [25]int // physical register of lane x+5y
	for i := range ph {
		ph[i] = i
	}
	for r := 0; r < 24; r++ {
		a := func(i int) VecPhysical { return Z[ph[i]] }
		C := Z[25:30]
		for x := 0; x < 5; x++ {
			VMOVDQA64(a(x), C[x])
			VPTERNLOGQ(xor3, a(x+10), a(x+5), C[x])
			VPTERNLOGQ(xor3, a(x+20), a(x+15), C[x])
		}
		// a(x, y) ^= C[x-1] ^ rol(C[x+1], 1); R for x+1 rotates into Z30/Z31, R0 into C1 (dead).
		R := map[int]VecPhysical{1: Z30, 2: Z31, 3: Z30, 4: Z31, 0: C[1]}
		for x := 0; x < 5; x++ {
			r := R[(x+1)%5]
			VPROLQ(U8(1), C[(x+1)%5], r)
			for y := 0; y < 5; y++ {
				VPTERNLOGQ(xor3, r, C[(x+4)%5], a(x+5*y))
			}
		}
		var nph [25]int
		for i := 0; i < 25; i++ {
			if rho[i] != 0 {
				VPROLQ(U8(rho[i]), a(i), a(i))
			}
			x, y := i%5, i/5
			nph[y+5*((2*x+3*y)%5)] = ph[i]
		}
		ph = nph
		t0, t1 := Z25, Z26
		for y := 0; y < 5; y++ {
			b := func(x int) VecPhysical { return a(x + 5*y) }
			VMOVDQA64(b(0), t0)
			VMOVDQA64(b(1), t1)
			VPTERNLOGQ(chi, b(2), b(1), b(0))
			VPTERNLOGQ(chi, b(3), b(2), b(1))
			VPTERNLOGQ(chi, b(4), b(3), b(2))
			VPTERNLOGQ(chi, t0, b(4), b(3))
			VPTERNLOGQ(chi, t1, t0, b(4))
		}
		VPXORQ_BCST(rcMem.Offset(8*r), a(0), a(0))
	}
	for i, v := range ph {
		if v != i {
			panic(fmt.Sprint("register map not identity after 24 rounds: ", ph))
		}
	}
	ADDQ(U32(136), base)
	DECQ(n)
	JNZ(LabelRef("block8"))
	for i := 0; i < 25; i++ {
		VMOVDQU64(Z[i], Mem{Base: st, Disp: 64 * i})
	}
	Label("done8")
	VZEROUPPER()
	RET()
}

const license = "// Copyright (C) 2026, Ava Labs, Inc. All rights reserved.\n// See the file LICENSE for licensing terms.\n\n"

func prependLicense(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, append([]byte(license), b...), 0o644); err != nil {
		panic(err)
	}
}
