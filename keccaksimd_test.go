// Copyright (C) 2026, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package keccaksimd

import (
	"bytes"
	"math/rand"
	"testing"

	"golang.org/x/crypto/sha3"
)

func ref(m []byte) (out [32]byte) {
	h := sha3.NewLegacyKeccak256()
	h.Write(m)
	h.Sum(out[:0])
	return out
}

func randMsg(r *rand.Rand, n int) []byte {
	m := make([]byte, n)
	r.Read(m)
	return m
}

// maxLen covers every tail length of the first three 136-byte blocks and the start of a fourth.
const maxLen = 3*rate + 2

func check(t *testing.T, msgs [][]byte) {
	t.Helper()
	out := make([][32]byte, len(msgs))
	HashBatch(out, msgs, nil)
	for i, m := range msgs {
		if want := ref(m); out[i] != want {
			t.Fatalf("message %d of %d, len %d: got %x, want %x", i, len(msgs), len(m), out[i], want)
		}
	}
}

// batchSizes are around the 4 and 8 lane widths, plus one large batch.
var batchSizes = []int{0, 1, 3, 4, 5, 7, 8, 9, 1000}

func TestEveryLengthEveryBatchSize(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < maxLen; n++ {
		for _, k := range batchSizes {
			if k == 1000 && n%17 != 0 {
				continue
			}
			msgs := make([][]byte, k)
			for i := range msgs {
				msgs[i] = randMsg(r, n)
			}
			check(t, msgs)
		}
	}
}

func TestMixedLengths(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for range 300 {
		msgs := make([][]byte, r.Intn(40))
		for i := range msgs {
			msgs[i] = randMsg(r, r.Intn(5*rate))
		}
		check(t, msgs)
	}
	all := make([][]byte, maxLen)
	for n := range all {
		all[n] = randMsg(r, n)
	}
	r.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	check(t, all)
}

func TestScratchReuse(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	var idx []int
	for range 20 {
		msgs := make([][]byte, 17)
		for i := range msgs {
			msgs[i] = randMsg(r, r.Intn(3*rate))
		}
		out := make([][32]byte, len(msgs))
		idx = HashBatch(out, msgs, idx)
		for i, m := range msgs {
			if out[i] != ref(m) {
				t.Fatalf("message %d", i)
			}
		}
	}
}

func TestInputUnchanged(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	msgs := make([][]byte, 9)
	saved := make([][]byte, 9)
	for i := range msgs {
		msgs[i] = randMsg(r, 2*rate+5)
		saved[i] = bytes.Clone(msgs[i])
	}
	check(t, msgs)
	for i := range msgs {
		if !bytes.Equal(msgs[i], saved[i]) {
			t.Fatalf("message %d changed", i)
		}
	}
}

func TestLengthMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	HashBatch(make([][32]byte, 1), nil, nil)
}

func TestGeneric(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	msgs := make([][]byte, maxLen)
	for n := range msgs {
		msgs[n] = randMsg(r, n)
	}
	out := make([][32]byte, len(msgs))
	hashGeneric(out, msgs)
	for n, m := range msgs {
		if out[n] != ref(m) {
			t.Fatalf("len %d", n)
		}
	}
}

// FuzzHashBatch splits the input into messages at the lengths taken from its first bytes.
func FuzzHashBatch(f *testing.F) {
	f.Add([]byte{3, 0, 200}, []byte("hello world"))
	f.Add([]byte{136, 135, 137, 0, 1}, bytes.Repeat([]byte{7}, 700))
	f.Fuzz(func(t *testing.T, lens, data []byte) {
		var msgs [][]byte
		for _, l := range lens {
			n := int(l) * 3
			if n > len(data) {
				n = len(data)
			}
			msgs = append(msgs, data[:n])
			data = data[n:]
		}
		check(t, msgs)
	})
}

func benchBatch(b *testing.B, n int) {
	r := rand.New(rand.NewSource(6))
	msgs := make([][]byte, 512)
	for i := range msgs {
		msgs[i] = randMsg(r, n)
	}
	out := make([][32]byte, len(msgs))
	b.SetBytes(int64(n * len(msgs)))
	for b.Loop() {
		HashBatch(out, msgs, nil)
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(msgs)), "ns/hash")
}

func benchRef(b *testing.B, n int) {
	r := rand.New(rand.NewSource(6))
	msgs := make([][]byte, 512)
	for i := range msgs {
		msgs[i] = randMsg(r, n)
	}
	out := make([][32]byte, len(msgs))
	b.SetBytes(int64(n * len(msgs)))
	for b.Loop() {
		hashGeneric(out, msgs)
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(msgs)), "ns/hash")
}

func BenchmarkHashBatch64(b *testing.B)  { benchBatch(b, 64) }
func BenchmarkHashBatch532(b *testing.B) { benchBatch(b, 532) }
func BenchmarkXCrypto64(b *testing.B)    { benchRef(b, 64) }
func BenchmarkXCrypto532(b *testing.B)   { benchRef(b, 532) }
