# Build and release engineering

See [source installation](INSTALLATION.md) for host setup and [offline packaging](OFFLINE.md) for static targets, commands, prerequisites and the redistribution gate.

Native C uses POSIX threads/barriers, fixed-width integer arithmetic and portable arrays; there are no x86 assembly/intrinsic requirements. Build flags preserve O2, fno-fast-math, pthread and libm; ffp-contract=off prevents architecture/compiler-dependent floating-point fusion. Conservative ISA flags are x86-64/generic or ARMv8-A. Native Go builds explicitly select linux and the detected architecture with GOAMD64=v1 / GOARM64=v8.0; inherited cross-build or newer-instruction settings do not leak into portable binaries. Cross compilation is evidence of build portability only. Unsupported architectures fail explicitly.

Build scripts must run on their target architecture/libc so every binary can be invoked before packaging. TARGET_ARCH/TARGET_LIBC can assert a target but cannot silently relabel another host. `CC` is a single compiler executable path. Every build uses the common Go selector (GOTOOLCHAIN=local). Release builds select existing Go offline; online acquisition is an initializer operation.

Validation:

```bash
GOTOOLCHAIN=local go test ./...
GOTOOLCHAIN=local go vet ./...
python3 tests/test_bootstrap.py
python3 tests/test_platform.py
python3 tests/test_native.py
bash -n init.sh
bash -n start.sh
find scripts -name '*.sh' -exec bash -n {} \;
# If available:
shellcheck init.sh start.sh scripts/bootstrap-common.sh scripts/install/*.sh
```

`tests/test_native.py` compiles an optimized known/corrupted pointer-cycle harness, testing work observability/integrity rather than matching an implementation string. Real kernel CPU/RAM runs are separate from this regression. Never treat successful compilation as measurement correctness proof.

`.github/workflows/compatibility.yml` attempts representative distro-container source builds/installation/bounded workloads and uploads JSON phase evidence. `python3 scripts/compatibility-test.py --workloads` can run in a disposable VM with local dependencies; `--install` explicitly authorizes missing-package installation. No raw devices or write-intensive host sweeps are used. RHEL/SLES testing requires legitimate repository environments outside the public matrix. Containers are not full-VM qualification.

`scripts/smoke-linux.sh` delegates to the same bounded qualification script. It is an explicit test command, never called by initialization. [Current evidence](COMPATIBILITY.md) distinguishes completed tests and outstanding targets.
