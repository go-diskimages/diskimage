# benchmarks — go-diskimages vs qemu-img

Reproducible performance-parity harness comparing **go-diskimages** against the
reference **qemu-img** tool on identical inputs. Results live in
[`../BENCHMARKS.md`](../BENCHMARKS.md).

This is a **standalone Go module** (its own `go.mod`). It is deliberately kept
out of the parent repo's `go test ./...` run, coverage gate, and 6-arch CI
matrix — `go list ./...` in the parent module does not descend into a nested
module.

## What it measures

| op | go-diskimages | reference |
|----|---------------|-----------|
| qcow2 read/decode | `qcow2.ConvertToRaw` | `qemu-img convert -O raw` |
| qcow2 create | `qcow2.Create` | `qemu-img create -f qcow2` |
| raw read | `raw` `Format.ToRaw` | `qemu-img convert -O raw` |
| dmg read/decode | `dmg.UnpackToTemp` | *(go-only — see note)* |

Correctness is verified **before** timing: the qcow2→raw decode is compared
byte-for-byte against qemu-img's output, the created qcow2 is opened by
`qemu-img info`, and the dmg decode is round-trip-checked against its source
raw.

**dmg has no qemu-img interop on macOS** (qemu-img cannot write dmg, hdiutil
refuses to convert a raw payload to UDZO, and qemu-img's dmg reader rejects the
blkx layout go-diskimages emits). The dmg row is therefore go-only and excluded
from the vs-qemu parity table.

## Running

The harness benchmarks the **local working trees** of the sibling repos via
`replace` directives (`../../qcow2`, `../../raw`, `../../dmg`), so those repos
must be checked out next to `diskimage/` under `go-diskimages/`.

```sh
cd benchmarks
GOWORK=off go run . -iters 7 -out ../BENCHMARKS.md
```

Flags: `-iters N` (best-of-N, default 5), `-qemu PATH` (default
`/opt/homebrew/bin/qemu-img`), `-out FILE`, `-keep` (keep scratch images),
`-dir DIR` (scratch dir).

`GOWORK=off` is required when a parent `go.work` is present.
