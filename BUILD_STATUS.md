# Development and compatibility status

The implementation includes automatic modes, safe owned disk datasets, platform-aware source initialization, runtime diagnostics, and architecture/libc-specific offline release tooling. Runtime and redistribution qualification remain separate.

Use [README.md](README.md) for current commands, [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) and [docs/compatibility.json](docs/compatibility.json) for actual test evidence, and [docs/OFFLINE.md](docs/OFFLINE.md) for candidate versus redistributable bundles. Previous validation notes describe their dated snapshots; they do not certify all current targets.

Do not treat Windows unit tests, cross compilation, container smoke checks or ELF inspection as universal Linux/ARM64 qualification. The recorded Alpine amd64 static build passed source installation, unit/regression checks and complete offline QUICK/STANDARD/MAX CPU/RAM/Disk tests in a fresh guest without network or development packages. Its exact source/notice/link-map inventory is documented in [docs/RELEASE-AUDIT.md](docs/RELEASE-AUDIT.md) and bound to the five binary hashes. Other build targets still need their own runtime and dependency reviews.
