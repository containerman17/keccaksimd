# keccaksimd

Batched Keccak-256 for Go. It hashes many messages at once, one message per SIMD lane.

The hash is the Ethereum one: Keccak-256 with the legacy padding byte `0x01`, the same as
`sha3.NewLegacyKeccak256` in `golang.org/x/crypto/sha3`.

## Use

```go
import "github.com/containerman17/keccaksimd"

out := make([][32]byte, len(msgs))
idx = keccaksimd.HashBatch(out, msgs, idx)
```

`HashBatch` sets `out[i]` to the hash of `msgs[i]`. It panics if the two slices differ in length.
Messages can have any lengths. Messages with the same number of full 136-byte blocks share
lanes, so a batch of equal-length messages is the fastest case. `idx` is scratch space; pass
`nil` or the slice that the last call returned. The package keeps no state and is safe for
concurrent use.

## CPU support

The package reads the CPU features at start with `golang.org/x/sys/cpu`:

- AVX-512F: 8 lanes. A remainder of 1 to 4 messages uses the 4-lane kernel.
- AVX2: 4 lanes.
- Anything else, and every architecture other than amd64: the package calls
  `golang.org/x/crypto/sha3` for each message. The results are the same.

## Speed

Intel Core i7-10700K (AVX2, no AVX-512), Go 1.26, batches of 512 equal-length messages,
`go test -bench . -count 3`. Lower is better.

| message length | HashBatch | x/crypto/sha3, one by one |
|---|---|---|
| 64 bytes | 153 ns per hash | 384 ns per hash |
| 532 bytes | 568 ns per hash | 1,458 ns per hash |

## Regenerate the assembly

The kernels in `keccak_amd64.s` and the declarations in `stub_amd64.go` are produced by
[avo](https://github.com/mmcloughlin/avo). The generator is the separate module in `asm/`.

```sh
go generate ./...
```

## Tests

```sh
go test ./...
go test -run x -fuzz FuzzHashBatch .
```

The tests compare every result with `golang.org/x/crypto/sha3`.

## License

Avalanche Ecosystem License, version 1.1. See [LICENSE](LICENSE).
