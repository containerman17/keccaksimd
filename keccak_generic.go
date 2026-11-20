// Copyright (C) 2026, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build !amd64

package keccaksimd

func hashMany(out [][32]byte, msgs [][]byte) { hashGeneric(out, msgs) }
