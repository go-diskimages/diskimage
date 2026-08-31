// Standalone module: kept out of the parent repo's `go test ./...` coverage
// gate and 6-arch CI matrix. Benchmarks only; not part of the library.
module github.com/go-diskimages/diskimage-benchmarks

go 1.26.4

require (
	github.com/go-diskimages/dmg v0.0.0-20260622110325-12b2a5087c73
	github.com/go-diskimages/qcow2 v0.1.0
	github.com/go-diskimages/raw v0.0.0-20260703072845-8efbfe3be17a
)

require github.com/go-compressions/lzfse v0.1.1-0.20260620062248-135e417e8ead // indirect

replace github.com/go-diskimages/qcow2 => ../../qcow2

replace github.com/go-diskimages/raw => ../../raw

replace github.com/go-diskimages/dmg => ../../dmg
