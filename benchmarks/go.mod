// Standalone module: kept out of the parent repo's `go test ./...` coverage
// gate and 6-arch CI matrix. Benchmarks only; not part of the library.
module github.com/go-diskimages/diskimage-benchmarks

go 1.26.4

require (
	github.com/go-diskimages/dmg v0.0.0-20260830075540-a868a60da7e6
	github.com/go-diskimages/qcow2 v0.1.0
	github.com/go-diskimages/raw v0.0.0-20260411083159-38eda0633563
)

require github.com/go-compressions/lzfse v0.1.1-0.20260620062248-135e417e8ead // indirect

replace github.com/go-diskimages/qcow2 => ../../qcow2

replace github.com/go-diskimages/raw => ../../raw

replace github.com/go-diskimages/dmg => ../../dmg
