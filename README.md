# violetBSD Image Writer

[![Build and release](https://github.com/FoenixLabs/violetBSD-image-writer/actions/workflows/release.yml/badge.svg)](https://github.com/FoenixLabs/violetBSD-image-writer/actions/workflows/release.yml)

[Download binaries](https://github.com/FoenixLabs/violetBSD-image-writer/releases)

A native desktop window with **Select image**, **Destination**, and **Write image**.
Choose a `.img` or `.img.gz`, select an unmounted external drive, then confirm
which drive will be erased. Writing provisions a fresh private entropy seed and
verifies the entire personalized image. No manual target-side entropy setup is
needed.

## Single-file downloads

The release builder produces one executable per platform:

| Platform | File | Native interface |
| --- | --- | --- |
| Linux x86-64 | `violetbsd-image-writer-linux-x86_64` | GTK 3 |
| Windows x86-64 | `violetbsd-image-writer-windows-x86_64.exe` | Win32 |
| macOS Intel and Apple Silicon | `violetbsd-image-writer-macos-universal` | Cocoa |

Each executable contains the Go application and its dependencies. No Python,
Go installation, companion application library, browser, or unpacked runtime is
required. **These are single-file executables, not fully static binaries:**
Linux uses system glibc and GTK 3; Windows and macOS use their system libraries.
Linux musl-only systems are not supported by this build. The GUI bindings use
`purego`, so the binaries can be cross-compiled with `CGO_ENABLED=0`; that flag
does not eliminate the system GUI libraries.

Download binaries and their SHA-256 checksums from the [Releases page](https://github.com/FoenixLabs/violetBSD-image-writer/releases).

## Run

- Linux: unmount the destination's partitions. Make the download executable with
  `chmod +x violetbsd-image-writer-linux-x86_64`, then launch it normally.
  GTK 3, polkit (`pkexec`) and a running desktop polkit authentication agent are
  required. After the erase confirmation, approve the system password dialog.
  The GUI stays unprivileged; only a headless disk-writing worker runs as root.
  If no prompt appears, check that your desktop's polkit agent is running.
- macOS: unmount the external disk in Disk Utility, make the download executable,
  and launch it normally from Terminal. Approve the macOS administrator password
  dialog after confirming the destination. The system authorization API buffers
  worker output, so progress may remain at the authorization message until the
  operation finishes; keep the app open. Native acceptance testing is pending.
- Windows: open the `.exe` and accept the administrator prompt. The executable
  manifest requests UAC elevation automatically.

Image selection and read-only drive discovery can run without administrator
privileges on Unix, but writing requires them. Windows builds request them at
startup. The default list is deliberately limited to external/removable media;
internal SATA disks are not offered. Disk identity and capacity are checked
again before writing. Only 512-byte logical sectors are supported.

macOS APFS destinations are excluded, including unmounted APFS containers, to
protect external boot disks. Windows destinations must support being taken
offline through Windows Storage; some removable USB flash devices do not and
are rejected. Windows leaves a selected disk offline after a write or failure;
disconnect it afterward. Linux refuses mounted filesystems, swap and mapped
children. The app does not automatically unmount Unix filesystems.

After successful writing and full verification, **Write image** becomes **Close**.
Click it to exit. Failed operations keep the write controls available for recovery.

## Entropy and image handling

Use a trusted, updated violetBSD release image with the reserved UFS entropy
file. Verify its published checksum before writing. The Go parser bounds every
filesystem read and rejects missing slots, unsafe ownership/permissions, links,
indirect blocks and unsupported geometry. The writer changes only the reserved
536-byte seed extent. `crypto/rand` supplies fresh host OS randomness for each
write; the seed is never logged or saved in a temporary personalized image.

Gzip inputs use bounded-memory streaming with no full-size temporary file. A
complete validation pass checks the compressed stream, expanded size and entropy
slot before the destination is opened. Metadata reads replay the stream as needed.
Writing then streams decompression directly to the drive and checks that the
source hash still matches. This costs additional reads but no 16 GiB extraction
space. Readback is checked against the SHA-256 of the personalized output, so it
will intentionally differ from the public source image's SHA-256.

The existing boot service consumes the seed before network services. Early
kernel messages before mounting root may still mention entropy. SSH configuration
is unchanged. A failed/interrupted write may leave an incomplete destination;
rewrite it before use. Never publish a written/personalized disk as a release.

## Verify entropy after booting violetBSD

After a normal multi-user boot, log in at the violetBSD console as root. These
checks do not display the private seed contents.

1. Confirm the kernel has enough credited entropy:

   ```sh
   sysctl kern.entropy.needed
   ```

   Expected output:

   ```text
   kern.entropy.needed = 0
   ```

2. Confirm the boot service actually loaded the disk seed:

   ```sh
   grep -F 'Loaded entropy from /var/db/entropy-file.' /var/run/rc.log
   ```

   Look for `Loaded entropy from /var/db/entropy-file.` in the result or on the
   boot console. This distinguishes loading the seed from merely having enough
   entropy from another source. The early pre-mount warning can still appear
   before this service runs.

3. Check the file's permissions, numeric owner/group, and length:

   ```sh
   stat -f '%Sp %u:%g %z' /var/db/entropy-file
   ```

   Expected for the current violetBSD image format:

   ```text
   -rw------- 0:0 536
   ```

   This means mode `0600`, root:wheel ownership, and a 536-byte seed file.
   File existence and length alone do not prove the seed supplied entropy;
   check the kernel value and successful load message as well.

The writer's full readback verification confirms that the newly generated seed
was written correctly **before boot**. During boot, `rndctl -L` consumes and
replaces the seed. Do not compare the post-boot file checksum with the original
file, or copy a seed from another installation. The new contents are expected.

If the value is nonzero, the file is absent, or the boot log reports a load
failure, collect these diagnostics:

```sh
grep -Ei 'random_seed|entropy|rndctl' /var/run/rc.log
ls -ln /var/db/entropy-file
mount
```

Check that the correct drive was written using this writer, that the root
filesystem is writable, and that the system completed a normal multi-user boot.
A generic raw-image writer does not replace the public zero-credit placeholder.
Do not generate SSH keys or enable cryptographic services until
`kern.entropy.needed` reports zero. Do not post or distribute the entropy file
itself. No manual seed-loading command is needed for a successful installation.

## Build and test

From this repository root, using Go 1.24 or newer:

```sh
go test ./...
go vet ./...
go run ./cmd/release -out dist
```

The release command pins the Windows resource generator, builds Linux and Windows
x86-64 plus both macOS architectures, and assembles/validates the universal Mach-O
file. It generates `violetbsd-image-writer.SHA256`. No C compiler, Python, Xcode,
MinGW, Node, or PyInstaller is needed. The first build downloads Go modules.

The read-only real-release test can be enabled with an absolute path:

```sh
VIOLET_TEST_IMAGE=/absolute/path/to/violetbsd-11.0-a2560me.img go test ./internal/writer -v
```

The executable also supports `--inspect IMAGE`, `--list`, `--version` and
`--licenses` (console output; the Windows release uses the GUI subsystem).
Third-party license notices are embedded in each executable and included in the
source archive.

## Validation status

Linux native window startup and layout inspected; real release-image seed extent
validated. Race-enabled tests cover the seed format/freshness, bounded UFS parser,
malformed images, partial writes, unchanged input, cross-chunk seed overlay,
full readback and corruption detection. Linux/macOS/Windows disk-filter fixtures
are tested on Linux. Short fuzz testing of the image parser passes.

macOS Intel/Apple Silicon and Windows x86-64 builds compile and their binary
formats are checked. They have **not** been run on those operating systems.
No real-device write test was performed for the Go edition on any platform.
Public distribution should follow native GUI/write acceptance tests and platform
signing/notarization as appropriate; these builds are not publisher-signed.

## Continuous builds and releases

GitHub Actions tests on Linux, macOS, and Windows, then cross-compiles all three
single-file artifacts with the pinned Go dependencies. Every successful push to
`main` publishes a prerelease named `build-<commit>`. Each download remains tied
to that source commit. A pushed `v*` tag publishes a versioned release instead.
Other branches and pull requests are built/tested without publishing releases.
Manual runs from `main` are also supported. Re-running a commit updates its
existing release assets. Failed tests or builds prevent publication.

The workflow uses GitHub's built-in `GITHUB_TOKEN`; no personal access token or
signing secret is required. Release creation requires Actions to be enabled and
the repository/organization to allow the workflow's `contents: write` permission.
Binaries are unsigned; compilation and automated tests do not constitute a
physical-drive acceptance test.

## Source layout

- `internal/writer`: seed generation, bounded UFS parser, streaming write/readback.
- `internal/disks`: OS-specific drive discovery, filtering, and device access.
- `internal/ui`: native GTK, Cocoa and Win32 windows plus the shared controller.
- `cmd/release`: cross-platform builds, Windows resources, universal Mac packaging.
- `THIRD_PARTY.txt`: bundled dependency license notices (also embedded in binaries).

User confirmed successful Linux writing and verification with version 0.2.2 on
2026-10-13. macOS/Windows physical-device acceptance remains pending.
