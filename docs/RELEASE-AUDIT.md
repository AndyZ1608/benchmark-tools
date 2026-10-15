# Recorded static-build dependency review

This review covers the 2026-10-14 Alpine 3.22.6 amd64 build only. It is a source, notice and link-input inventory, not certification of other builds or distributions. Each packaged audit records the exact five binary hashes. A changed binary requires a new matching record.

## Linked components

| Component | Recorded version | Retained material |
|---|---|---|
| fio | 3.39 | GPLv2 license, complete pinned upstream source, configure output and build script |
| stress-ng | 0.18.10 | GPLv2 license, complete pinned upstream source and its per-file notices, feature configuration and build script |
| iperf3 | 3.17.1 | BSD license, complete pinned source including embedded MIT cJSON, standalone cJSON notice and build configuration |
| musl | 1.2.5-r12 | Complete COPYRIGHT/attributions, source archive, exact Alpine recipe and patches |
| libaio | 0.3.113-r2 | LGPL 2.1 notice, complete source, exact recipe/patches; complete fio/stress-ng sources and rebuild instructions |
| zlib | 1.3.2-r1 | zlib notice, complete source, exact Alpine recipe and patch |
| GCC runtime | 14.2.0-r6 | GPLv3 and GCC Runtime Library Exception 3.1, libgcc/libatomic source headers and immutable upstream references |
| Go runtime | 1.23.12 | LICENSE/PATENTS and all source-tree LICENSE/COPYING/PATENTS/NOTICE files, including math and vendored packages |

The engine link maps enumerate libaio, zlib, libatomic, libgcc/libgcc_eh and musl. musl supplies libc/libm/pthread/rt/dl/crypt and the musl-dev libssp_nonshared shim; the latter's source is retained. No OpenSSL library is linked. The Go CLI uses CGO_ENABLED=0. The native compute kernel links the same musl/compiler runtime. Static ELF checks reject interpreters and shared-library dependencies.

The shipped GPL engines are unmodified upstream sources, compiled with recorded configuration. Their full sources accompany the binaries, rather than a promise of a later source download. Additional source-level notices remain intact inside those archives. libaio users can modify/relink using the complete library and engine sources: [LGPL 2.1 section 6](https://www.gnu.org/licenses/old-licenses/lgpl-2.1.html) permits corresponding work in source and/or object form. GCC was used without proprietary compilation plugins; its runtime permissions are recorded with [Exception 3.1](https://www.gnu.org/licenses/gcc-exception-3.1.html). No license terms restrict modification or reverse engineering of the included open-source programs.

## Rebuilding third-party engines

On a matching build VM with the recorded development packages (or rebuilt libraries from the included runtime recipes), extract the archive and run:

```bash
cd vmbench/LICENSES/rebuild
mkdir -p third-party LICENSES
cp ../fio.tar.gz ../stress-ng.tar.gz ../iperf3.tar.gz third-party/
cp -R ../fio ../stress-ng ../iperf3 LICENSES/
bash scripts/build-engines.sh
```

The included runtime source index records immutable recipe commits and source SHA-256. Apply the corresponding recipe patches when rebuilding a library. Rebuilt engines are in vendor-engines. Replace an engine only in a copy of the bundle, then regenerate its release manifest using the source checkout's packaging tools; doctor deliberately rejects stale hashes. Modification never requires disabling an engine signature check.

## Boundaries

The audit record is a technical review by Codex of the recorded source/build inputs. It does not grant rights beyond the included licenses or claim broader platform readiness. A different compiler, runtime revision, optional static library, build target or binary hash requires another inventory. glibc and ARM64 redistribution/runtime qualification are not supplied by this record.
