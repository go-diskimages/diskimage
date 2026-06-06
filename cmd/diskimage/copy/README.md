diskimage copy — copy files to/from a disk image
=================================================

Usage
-----

- Copy a local file into the image:

```
diskimage copy --file disk.img --to-image ./local.txt /path/in/image/remote.txt
```

- Copy a file from the image to the local host:

```
diskimage copy --file disk.img --from-image /path/in/image/remote.txt ./local.txt
```

Flags
-----

- `--file, -f` : path to the disk image file (required)
- `--filesystem` : optional filesystem type (ext4, fat32, btrfs, xfs, zfs, exfat). If omitted, it is auto-detected.
- `--part` : 0-based partition index to open (default 0)
 - `--part` : 0-based partition index to open (use `-1` to auto-detect, `0` for first partition)
- `--to-image` : copy from host into image (host-src image-dst)
- `--from-image` : copy from image to host (image-src host-dst)

Notes
-----

- Tests in this repository exercise the command against real on-disk images (temporary files) to ensure behavior matches real-world usage. The FAT32 formatter is used in tests because of its small footprint and fast formatting time.
