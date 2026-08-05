Integration test resource recommendations

This document lists minimal recommended image sizes and notes for running the
`go-diskimages/diskimage` integration stress tests that exercise multiple filesystem
implementations.

Recommended minimal image sizes (used by the stress test):

- fat32, exfat: 4 MiB
- ext4: 20 MiB
- btrfs: 8 GiB
- xfs: 8 GiB
- zfs: 8 GiB

Notes and rationale:

- "Heavy" filesystems (btrfs, xfs, zfs) allocate significant metadata structures
  during formatting and during repeated create/delete operations; small images
  can exhaust metadata structures (leaf full / no free inode / pool full).
  Tests that perform many create/delete/rename iterations therefore require
  substantially larger images to avoid spurious failures.

- The stress test by default uses the sizes above. If you do not have enough
  disk space or RAM to run heavy tests, set `DISKIMAGE_STRESS_ITERS` to a
  smaller value (or run only a subset of filesystems).

- Partitioned images: some filesystem drivers have limitations when used inside
  a partitioned image (e.g., ZFS drivers may require special handling). The
  integration tests currently try both bare images and images with MBR/GPT
  partition tables; if you observe driver-specific errors for partitioned
  images, prefer re-running with a larger `DISKIMAGE_STRESS_ITERS` reduction
  and larger image sizes, or run the single-FS targeted test for diagnosis.

- Disk space: running the full matrix (all filesystems × partitions) at the
  recommended sizes may require tens of GiB of temporary disk space while
  tests are running. Ensure your test host has sufficient free space.

- CI: Because heavy FS tests are resource-intensive, consider gating them in
  CI (run nightly or on-demand) or using smaller iteration counts for PR runs
  while keeping the large-image tests for release verification.
