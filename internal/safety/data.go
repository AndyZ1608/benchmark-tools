package safety

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

const ReserveBytes uint64 = 1 << 30
const metadataReserve uint64 = 1 << 20
const dataChunk = 1 << 20

type Plan struct {
	FileBytes, TotalBytes, AvailableBytes, ReserveBytes uint64
	Jobs                                                int
}

// PlanSpace includes all jobs and headroom, before any workspace or file is created.
func PlanSpace(free uint64, size int64, jobs int, automatic bool) (Plan, error) {
	p := Plan{AvailableBytes: free, ReserveBytes: ReserveBytes, Jobs: jobs}
	if size <= 0 || jobs < 1 || jobs > 128 {
		return p, fmt.Errorf("invalid disk dataset: size=%d jobs=%d", size, jobs)
	}
	n := uint64(size)
	if n > math.MaxUint64/uint64(jobs) {
		return p, fmt.Errorf("disk dataset size overflow")
	}
	if automatic && free > ReserveBytes+metadataReserve {
		cap := (free - ReserveBytes - metadataReserve) / uint64(jobs)
		cap = cap / (1 << 20) * (1 << 20)
		if cap < n {
			if cap < 16<<20 {
				return p, fmt.Errorf("insufficient disk space for automatic sizing: available %d bytes; reserved %d bytes; minimum 16MiB per job", free, ReserveBytes)
			}
			n = cap
		}
	}
	p.FileBytes = n
	p.TotalBytes = n * uint64(jobs)
	if free < ReserveBytes+metadataReserve || p.TotalBytes > free-ReserveBytes-metadataReserve {
		return p, fmt.Errorf("insufficient disk space: required %d bytes for %d jobs; available %d bytes; reserved free space %d bytes plus %d bytes metadata", p.TotalBytes, jobs, free, ReserveBytes, metadataReserve)
	}
	return p, nil
}
func dataStream(seed uint64, job int) cipher.Stream {
	var keyInput [32]byte
	copy(keyInput[:], "VMBench-data-v1")
	binary.LittleEndian.PutUint64(keyInput[16:24], seed)
	binary.LittleEndian.PutUint64(keyInput[24:], uint64(job))
	key := sha256.Sum256(keyInput[:])
	block, _ := aes.NewCipher(key[:])
	return cipher.NewCTR(block, make([]byte, aes.BlockSize))
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if n < 0 || n > len(b) {
			return io.ErrShortWrite
		}
		if n > 0 {
			b = b[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// prepareData writes every byte, fsyncs, then verifies the complete deterministic stream.
// Two reusable 1MiB buffers bound memory regardless of the requested dataset size.
func prepareData(ctx context.Context, f *os.File, size int64, seed uint64, job int, guards ...func(int) error) ([32]byte, error) {
	var digest [32]byte
	stream := dataStream(seed, job)
	expected, actual := make([]byte, dataChunk), make([]byte, dataChunk)
	hash := sha256.New()
	for offset := int64(0); offset < size; {
		if err := ctx.Err(); err != nil {
			return digest, err
		}
		n := int64(len(expected))
		if n > size-offset {
			n = size - offset
		}
		for _, guard := range guards {
			if err := guard(int(n)); err != nil {
				return digest, err
			}
		}
		chunk := expected[:n]
		clear(chunk)
		stream.XORKeyStream(chunk, chunk)
		if err := writeAll(f, chunk); err != nil {
			return digest, err
		}
		hash.Write(chunk)
		offset += n
	}
	if err := f.Sync(); err != nil {
		return digest, fmt.Errorf("synchronize data: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		return digest, err
	}
	if info.Size() != size {
		return digest, fmt.Errorf("prepared size %d differs from requested %d", info.Size(), size)
	}
	stream = dataStream(seed, job)
	for offset := int64(0); offset < size; {
		if err := ctx.Err(); err != nil {
			return digest, err
		}
		n := int64(len(expected))
		if n > size-offset {
			n = size - offset
		}
		chunk := expected[:n]
		clear(chunk)
		stream.XORKeyStream(chunk, chunk)
		got, err := f.ReadAt(actual[:n], offset)
		if err != nil || int64(got) != n {
			return digest, fmt.Errorf("verify data at offset %d: %w", offset, errors.Join(err, io.ErrUnexpectedEOF))
		}
		for i := range chunk {
			if actual[i] != chunk[i] {
				return digest, fmt.Errorf("data verification mismatch at offset %d", offset+int64(i))
			}
		}
		offset += n
	}
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}
