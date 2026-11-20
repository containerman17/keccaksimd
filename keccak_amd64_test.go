// Copyright (C) 2026, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package keccaksimd

import (
	"math/rand"
	"testing"
)

// TestLanes runs each kernel the CPU has on its own, for every length and every lane fill.
func TestLanes(t *testing.T) {
	for _, w := range []int{4, 8} {
		if w == 4 && !hasAVX2 || w == 8 && !hasAVX512 {
			t.Logf("no %d-lane kernel on this CPU", w)
			continue
		}
		r := rand.New(rand.NewSource(int64(w)))
		for n := 0; n < maxLen; n++ {
			for k := 1; k <= w+1; k++ {
				msgs := make([][]byte, k)
				for i := range msgs {
					msgs[i] = randMsg(r, n)
				}
				out := make([][32]byte, k)
				hashLanes(out, msgs, w)
				for i, m := range msgs {
					if out[i] != ref(m) {
						t.Fatalf("w %d len %d msg %d of %d", w, n, i, k)
					}
				}
			}
		}
	}
}

// TestPaths forces each hashMany branch the CPU can run, including the generic one.
func TestPaths(t *testing.T) {
	avx2, avx512 := hasAVX2, hasAVX512
	defer func() { hasAVX2, hasAVX512 = avx2, avx512 }()
	r := rand.New(rand.NewSource(9))
	for _, p := range [][2]bool{{false, false}, {avx2, false}, {avx2, avx512}} {
		hasAVX2, hasAVX512 = p[0], p[1]
		for n := 0; n < maxLen; n += 7 {
			for _, k := range []int{1, 4, 5, 8, 12, 13, 16} {
				msgs := make([][]byte, k)
				for i := range msgs {
					msgs[i] = randMsg(r, n)
				}
				check(t, msgs)
			}
		}
	}
}
