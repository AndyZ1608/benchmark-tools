package safety

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const oPath = 0x200000
const atRemoveDir = 0x200
const ownerName = ".vmbench-owner"

type trackedFile struct {
	name     string
	file     *os.File
	stat     syscall.Stat_t
	removed  bool
	reserved bool
}
type Target struct {
	Dir        string
	Files      []string
	Plan       Plan
	fd, parent int
	spare      int
	name       string
	identity   syscall.Stat_t
	owned      []*trackedFile
	manifest   *trackedFile
	closed     bool
	closeErr   error
}

func openDirectory(path string) (string, int, error) {
	if path == "" {
		return "", -1, fmt.Errorf("explicit --path directory required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", -1, fmt.Errorf("resolve target directory %q: %w", path, err)
	}
	for _, prefix := range []string{"/dev", "/sys", "/proc"} {
		if absolute == prefix || strings.HasPrefix(absolute, prefix+"/") {
			return "", -1, fmt.Errorf("target directory %q is a rejected pseudo-filesystem path", absolute)
		}
	}
	// Walk each component relative to the prior handle. No ancestor symlink is followed.
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return "", -1, fmt.Errorf("open filesystem root for target %q: %w", absolute, err)
	}
	for _, part := range strings.Split(strings.TrimPrefix(absolute, "/"), "/") {
		if part == "" {
			continue
		}
		next, e := syscall.Openat(fd, part, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		syscall.Close(fd)
		if e != nil {
			return "", -1, fmt.Errorf("open target directory %q (component %q): %w; directory must exist and contain no symlinks", absolute, part, e)
		}
		fd = next
	}
	return absolute, fd, nil
}
func ValidateDirectory(path string) (map[string]any, error) {
	absolute, fd, err := openDirectory(path)
	if err != nil {
		return nil, err
	}
	defer syscall.Close(fd)
	var fs syscall.Statfs_t
	if err = syscall.Fstatfs(fd, &fs); err != nil {
		return nil, fmt.Errorf("inspect target filesystem %q: %w", absolute, err)
	}
	if pseudoFilesystem(fs.Type) {
		return nil, fmt.Errorf("target directory %q is on a rejected pseudo-filesystem (0x%x)", absolute, fs.Type)
	}
	if fs.Flags&1 != 0 {
		return nil, fmt.Errorf("target directory %q is on a read-only filesystem", absolute)
	}
	if fs.Bsize <= 0 || uint64(fs.Bavail) > ^uint64(0)/uint64(fs.Bsize) {
		return nil, fmt.Errorf("invalid filesystem capacity for target %q", absolute)
	}
	var s, root syscall.Stat_t
	if err = syscall.Fstat(fd, &s); err != nil {
		return nil, fmt.Errorf("inspect target directory %q: %w", absolute, err)
	}
	if err = syscall.Faccessat(fd, ".", 3, 0x200); err != nil {
		return nil, fmt.Errorf("target directory %q is not writable/searchable: %w", absolute, err)
	}
	if s.Mode&0222 == 0 {
		return nil, fmt.Errorf("target directory %q has no write permission bits", absolute)
	}
	_ = syscall.Stat("/", &root)
	return map[string]any{"path": absolute, "available_bytes": uint64(fs.Bavail) * uint64(fs.Bsize), "filesystem_magic": fmt.Sprintf("0x%x", fs.Type), "system_filesystem": s.Dev == root.Dev}, nil
}
func Prepare(path, id string, size int64, jobs int) (*Target, error) {
	return PrepareSized(path, id, size, jobs, false)
}
func PrepareSized(path, id string, size int64, jobs int, automatic bool) (_ *Target, err error) {
	return PrepareReserved(path, id, size, jobs, automatic, ReserveBytes)
}
func PrepareReserved(path, id string, size int64, jobs int, automatic bool, reserve uint64) (_ *Target, err error) {
	absolute, fd, err := openDirectory(path)
	if err != nil {
		return nil, err
	}
	t := &Target{Dir: absolute, fd: -1, parent: fd, spare: -1}
	defer func() {
		if err != nil {
			err = errors.Join(err, t.Close())
		}
	}()
	var fs syscall.Statfs_t
	if err = syscall.Fstatfs(fd, &fs); err != nil {
		return nil, fmt.Errorf("inspect target filesystem %q: %w", absolute, err)
	}
	if pseudoFilesystem(fs.Type) {
		return nil, fmt.Errorf("target directory %q is on a rejected pseudo-filesystem (0x%x)", absolute, fs.Type)
	}
	if fs.Flags&1 != 0 {
		return nil, fmt.Errorf("target directory %q is on a read-only filesystem", absolute)
	}
	var parentStat syscall.Stat_t
	if err = syscall.Fstat(fd, &parentStat); err != nil {
		return nil, fmt.Errorf("inspect target directory %q: %w", absolute, err)
	}
	if err = syscall.Faccessat(fd, ".", 3, 0x200); err != nil {
		return nil, fmt.Errorf("target directory %q is not writable/searchable: %w", absolute, err)
	}
	if parentStat.Mode&0222 == 0 {
		return nil, fmt.Errorf("target directory %q has no write permission bits", absolute)
	}
	if fs.Bsize <= 0 || uint64(fs.Bavail) > ^uint64(0)/uint64(fs.Bsize) {
		return nil, fmt.Errorf("invalid filesystem capacity for target %q", absolute)
	}
	t.Plan, err = PlanSpaceReserved(uint64(fs.Bavail)*uint64(fs.Bsize), size, jobs, automatic, reserve)
	if err != nil {
		return nil, fmt.Errorf("target directory %q: %w", absolute, err)
	}
	if id == "" {
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return nil, fmt.Errorf("generate private workspace ID: %w", err)
		}
		id = hex.EncodeToString(nonce[:])
	}
	if len(id) > 64 || strings.ContainsAny(id, "/\\") || id == "." || id == ".." {
		return nil, fmt.Errorf("invalid workspace run ID %q", id)
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return nil, fmt.Errorf("invalid workspace run ID %q", id)
		}
	}
	t.name = ".vmbench-" + id
	if err = syscall.Mkdirat(fd, t.name, 0700); err != nil {
		t.name = ""
		return nil, fmt.Errorf("create private workspace under %q: %w", absolute, err)
	}
	t.Dir = filepath.Join(absolute, t.name)
	t.fd, err = syscall.Openat(fd, t.name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open created workspace %q: %w; cleanup identity cannot be proven", t.Dir, err)
	}
	if err = syscall.Fstat(t.fd, &t.identity); err != nil {
		return nil, fmt.Errorf("record workspace identity %q: %w", t.Dir, err)
	}
	if t.identity.Uid != uint32(os.Geteuid()) || t.identity.Mode&syscall.S_IFMT != syscall.S_IFDIR {
		return nil, fmt.Errorf("workspace ownership/type changed: %q", t.Dir)
	}
	if err = syscall.Fchmod(t.fd, 0700); err != nil {
		return nil, fmt.Errorf("restrict workspace permissions %q: %w", t.Dir, err)
	}
	spare, _, reserveErr := syscall.Syscall(syscall.SYS_FCNTL, uintptr(t.fd), syscall.F_DUPFD_CLOEXEC, 0)
	if reserveErr != 0 {
		return nil, fmt.Errorf("reserve cleanup file descriptor for workspace %q: %w", t.Dir, reserveErr)
	}
	t.spare = int(spare)
	t.manifest, err = t.createFile(ownerName)
	if err != nil {
		return nil, err
	}
	if err = t.record("VMBENCH-OWNERSHIP-v1\nrun=%s uid=%d pid=%d created=%s dev=%d ino=%d bytes_per_job=%d jobs=%d\n", id, os.Geteuid(), os.Getpid(), time.Now().UTC().Format(time.RFC3339), t.identity.Dev, t.identity.Ino, t.Plan.FileBytes, jobs); err != nil {
		return nil, err
	}
	for i := 0; i < jobs; i++ {
		name := fmt.Sprintf("job-%03d.dat", i+1)
		f, e := t.createFile(name)
		if e != nil {
			return nil, e
		}
		t.owned = append(t.owned, f)
		t.Files = append(t.Files, name)
		if err = t.checkHeadroom(t.Plan.FileBytes); err != nil {
			return nil, err
		}
		allocation := syscall.Fallocate(int(f.file.Fd()), 0, 0, int64(t.Plan.FileBytes))
		if allocation == nil {
			f.reserved = true
		} else if !errors.Is(allocation, syscall.EOPNOTSUPP) && !errors.Is(allocation, syscall.ENOSYS) && !errors.Is(allocation, syscall.EINVAL) {
			return nil, fmt.Errorf("reserve space for test file %q: %w", filepath.Join(t.Dir, name), allocation)
		}
		if err = t.record("file=%s dev=%d ino=%d uid=%d\n", name, f.stat.Dev, f.stat.Ino, f.stat.Uid); err != nil {
			return nil, err
		}
	}
	if err = syscall.Fsync(t.fd); err != nil {
		return nil, fmt.Errorf("synchronize workspace %q: %w", t.Dir, err)
	}
	if err = syscall.Fsync(t.parent); err != nil {
		return nil, fmt.Errorf("synchronize target directory %q: %w", absolute, err)
	}
	return t, nil
}
func (t *Target) createFile(name string) (*trackedFile, error) {
	fd, err := syscall.Openat(t.fd, name, syscall.O_CREAT|syscall.O_EXCL|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, fmt.Errorf("create private file %q: %w", filepath.Join(t.Dir, name), err)
	}
	f := &trackedFile{name: name, file: os.NewFile(uintptr(fd), name)}
	if err = syscall.Fstat(fd, &f.stat); err == nil {
		err = syscall.Fchmod(fd, 0600)
	}
	if err != nil {
		f.file.Close()
		return nil, fmt.Errorf("record private file identity %q: %w", filepath.Join(t.Dir, name), err)
	}
	return f, nil
}
func (t *Target) record(format string, args ...any) error {
	if t.manifest == nil {
		return nil
	}
	if _, err := fmt.Fprintf(t.manifest.file, format, args...); err != nil {
		return fmt.Errorf("write ownership metadata %q: %w", t.Dir, err)
	}
	if err := t.manifest.file.Sync(); err != nil {
		return fmt.Errorf("sync ownership metadata %q: %w", t.Dir, err)
	}
	return nil
}
func (t *Target) Fill(ctx context.Context, seed uint64, progress func(int, int)) error {
	if err := t.record("seed=%d phase=preparing\n", seed); err != nil {
		return err
	}
	for i, f := range t.owned {
		if err := t.verifyFile(f); err != nil {
			return err
		}
		digest, err := prepareData(ctx, f.file, int64(t.Plan.FileBytes), seed, i, func(n int) error {
			extra := uint64(n)
			if f.reserved {
				extra = 0
			}
			return t.checkHeadroom(extra)
		})
		if err != nil {
			return fmt.Errorf("prepare test file %q: %w", filepath.Join(t.Dir, f.name), err)
		}
		// Filesystems supporting SEEK_HOLE must not report a hole inside the dataset.
		hole, e := syscall.Seek(int(f.file.Fd()), 0, 4)
		if e == nil && hole < int64(t.Plan.FileBytes) {
			return fmt.Errorf("prepared file %q contains an unwritten hole at offset %d", filepath.Join(t.Dir, f.name), hole)
		}
		if e != nil && !errors.Is(e, syscall.EINVAL) && !errors.Is(e, syscall.ENOTSUP) {
			return fmt.Errorf("check prepared allocation %q: %w", filepath.Join(t.Dir, f.name), e)
		}
		if err = t.record("verified=%s sha256=%x phase=prepared\n", f.name, digest); err != nil {
			return err
		}
		// Best effort, file-specific cache eviction after full verification. No global cache drop.
		_, _, _ = syscall.Syscall6(syscall.SYS_FADVISE64, f.file.Fd(), 0, uintptr(t.Plan.FileBytes), 4, 0, 0)
		if progress != nil {
			progress(i+1, len(t.owned))
		}
	}
	return t.Validate()
}
func sameIdentity(a, b syscall.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Uid == b.Uid && a.Mode&syscall.S_IFMT == b.Mode&syscall.S_IFMT
}
func statAt(fd int, name string) (syscall.Stat_t, error) {
	var st syscall.Stat_t
	f, err := syscall.Openat(fd, name, oPath|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return st, err
	}
	defer syscall.Close(f)
	err = syscall.Fstat(f, &st)
	return st, err
}
func (t *Target) verifyFile(f *trackedFile) error {
	current, err := statAt(t.fd, f.name)
	if err != nil {
		return fmt.Errorf("verify private file %q: %w", filepath.Join(t.Dir, f.name), err)
	}
	if !sameIdentity(current, f.stat) || current.Mode&syscall.S_IFMT != syscall.S_IFREG || current.Nlink != 1 || current.Mode&0777 != 0600 {
		return fmt.Errorf("private file ownership/identity changed at %q; refusing access or deletion", filepath.Join(t.Dir, f.name))
	}
	return nil
}
func (t *Target) Validate() error {
	st, err := statAt(t.parent, t.name)
	if err != nil {
		return fmt.Errorf("verify workspace %q: %w", t.Dir, err)
	}
	if !sameIdentity(st, t.identity) || st.Mode&syscall.S_IFMT != syscall.S_IFDIR || st.Mode&0777 != 0700 {
		return fmt.Errorf("workspace ownership/identity/permissions changed at %q; refusing access or cleanup", t.Dir)
	}
	for _, f := range t.owned {
		if err = t.verifyFile(f); err != nil {
			return err
		}
		var s syscall.Stat_t
		if err = syscall.Fstat(int(f.file.Fd()), &s); err != nil {
			return fmt.Errorf("verify retained file handle %q: %w", filepath.Join(t.Dir, f.name), err)
		}
		if uint64(s.Size) != t.Plan.FileBytes {
			return fmt.Errorf("prepared size changed at %q", filepath.Join(t.Dir, f.name))
		}
	}
	return nil
}

// Paths refer to the retained parent handle; EngineFiles uses child-local handles instead.
func (t *Target) Paths() []string {
	out := []string{}
	for _, f := range t.owned {
		out = append(out, fmt.Sprintf("/proc/%d/fd/%d/%s", os.Getpid(), t.fd, f.name))
	}
	return out
}
func (t *Target) EngineFiles() []*os.File {
	out := []*os.File{}
	for _, f := range t.owned {
		out = append(out, f.file)
	}
	return out
}
func unlinkDirectory(fd int, name string) error {
	p, err := syscall.BytePtrFromString(name)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall(syscall.SYS_UNLINKAT, uintptr(fd), uintptr(unsafe.Pointer(p)), atRemoveDir)
	if e != 0 {
		return e
	}
	return nil
}
func (t *Target) Close() (result error) {
	if t.closed {
		return t.closeErr
	}
	t.closed = true
	// Preserve a slot for identity checks even after an EMFILE preparation failure.
	if t.spare >= 0 {
		syscall.Close(t.spare)
		t.spare = -1
	}
	defer func() {
		releases := []error{}
		for _, f := range t.owned {
			if err := f.file.Close(); err != nil {
				releases = append(releases, fmt.Errorf("close retained test file %q: %w", filepath.Join(t.Dir, f.name), err))
			}
		}
		if t.manifest != nil {
			if err := t.manifest.file.Close(); err != nil {
				releases = append(releases, fmt.Errorf("close ownership metadata %q: %w", t.Dir, err))
			}
		}
		if len(releases) > 0 {
			result = errors.Join(result, errors.Join(releases...))
			t.closeErr = result
		}
		if t.fd >= 0 {
			syscall.Close(t.fd)
			t.fd = -1
		}
		if t.parent >= 0 {
			syscall.Close(t.parent)
			t.parent = -1
		}
	}()
	if t.name == "" {
		return nil
	}
	if t.fd < 0 || t.identity.Ino == 0 {
		t.closeErr = fmt.Errorf("cleanup incomplete; identity unavailable for created workspace %q", t.Dir)
		return t.closeErr
	}
	directory, err := statAt(t.parent, t.name)
	if err != nil || !sameIdentity(directory, t.identity) || directory.Mode&0777 != 0700 {
		t.closeErr = fmt.Errorf("cleanup refused: workspace identity changed at %q", t.Dir)
		return t.closeErr
	}
	errs := []error{}
	for _, f := range t.owned {
		if err = t.verifyFile(f); err != nil {
			errs = append(errs, err)
			continue
		}
		if err = syscall.Unlinkat(t.fd, f.name); err != nil {
			errs = append(errs, fmt.Errorf("remove verified private file %q: %w", filepath.Join(t.Dir, f.name), err))
		} else {
			f.removed = true
		}
	}
	// Keep ownership metadata whenever any artifact or unrelated file remains.
	duplicate, _, e := syscall.Syscall(syscall.SYS_FCNTL, uintptr(t.fd), syscall.F_DUPFD_CLOEXEC, 0)
	if e != 0 {
		errs = append(errs, e)
	} else {
		directoryFile := os.NewFile(duplicate, "workspace-list")
		names, e := directoryFile.Readdirnames(-1)
		directoryFile.Close()
		if e != nil {
			errs = append(errs, e)
		} else {
			for _, name := range names {
				if name != ownerName {
					errs = append(errs, fmt.Errorf("unremoved/untracked entry %q; refusing recursive cleanup", filepath.Join(t.Dir, name)))
				}
			}
		}
	}
	if len(errs) == 0 && t.manifest != nil {
		if err = t.verifyFile(t.manifest); err != nil {
			errs = append(errs, err)
		} else if err = syscall.Unlinkat(t.fd, ownerName); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		directory, err = statAt(t.parent, t.name)
		if err != nil || !sameIdentity(directory, t.identity) || directory.Mode&0777 != 0700 {
			errs = append(errs, fmt.Errorf("workspace identity changed before directory removal: %q", t.Dir))
		} else if err = unlinkDirectory(t.parent, t.name); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		t.closeErr = fmt.Errorf("temporary file cleanup incomplete; remaining workspace %q: %w", t.Dir, errors.Join(errs...))
	}
	return t.closeErr
}

func pseudoFilesystem(magic int64) bool {
	switch magic {
	case 0x9fa0, 0x62656572, 0x1cd1, 0x27e0eb, 0x63677270, 0x64626720, 0x73636673:
		return true
	}
	return false
}

func (t *Target) checkHeadroom(extra uint64) error {
	var fs syscall.Statfs_t
	if err := syscall.Fstatfs(t.fd, &fs); err != nil {
		return fmt.Errorf("recheck available space in workspace %q: %w", t.Dir, err)
	}
	if fs.Bsize <= 0 || uint64(fs.Bavail) > ^uint64(0)/uint64(fs.Bsize) {
		return fmt.Errorf("invalid filesystem capacity in workspace %q", t.Dir)
	}
	free := uint64(fs.Bavail) * uint64(fs.Bsize)
	if extra > ^uint64(0)-t.Plan.ReserveBytes-metadataReserve || free < t.Plan.ReserveBytes+metadataReserve+extra {
		return fmt.Errorf("free space dropped below required reserve in workspace %q: available %d bytes; reserved %d bytes", t.Dir, free, t.Plan.ReserveBytes)
	}
	return nil
}
