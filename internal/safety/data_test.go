package safety

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"testing"
)

func TestDatasetPlanAndReserve(t *testing.T) {
	p, err := PlanSpace(12<<30, 2<<30, 4, false)
	if err != nil || p.FileBytes != 2<<30 || p.TotalBytes != 8<<30 || p.ReserveBytes != 1<<30 {
		t.Fatal(p, err)
	}
	for _, test := range []struct {
		free uint64
		size int64
		jobs int
	}{{5 << 30, 2 << 30, 4}, {1 << 30, 4096, 1}, {10 << 30, 0, 1}, {10 << 30, 4096, 0}, {math.MaxUint64, math.MaxInt64, 128}} {
		if _, err = PlanSpace(test.free, test.size, test.jobs, false); err == nil {
			t.Fatal("unsafe plan accepted", test)
		}
	}
	p, err = PlanSpace(3<<30, 1<<30, 4, true)
	if err != nil || p.TotalBytes+ReserveBytes+metadataReserve > p.AvailableBytes || p.FileBytes%(1<<20) != 0 {
		t.Fatal(p, err)
	}
}
func TestStrongerConfiguredReserveCannotBeConsumed(t *testing.T) {
	if _, err := PlanSpaceReserved(4<<30, 2<<30, 1, false, 3<<30); err == nil {
		t.Fatal("configured reserve was ignored")
	}
	if _, err := PlanSpaceReserved(10<<30, 4096, 1, false, ReserveBytes-1); err == nil {
		t.Fatal("reserve below 1GiB accepted")
	}
	p, err := PlanSpaceReserved(8<<30, 2<<30, 2, false, 3<<30)
	if err != nil || p.ReserveBytes != 3<<30 || p.TotalBytes != 4<<30 {
		t.Fatal(p, err)
	}
}

type partialWriter struct {
	bytes.Buffer
	zero bool
	fail bool
}

func (w *partialWriter) Write(b []byte) (int, error) {
	if w.zero {
		return 0, nil
	}
	if w.fail {
		return 0, errors.New("write fixture failure")
	}
	if len(b) > 7 {
		b = b[:7]
	}
	return w.Buffer.Write(b)
}
func TestPreparationHandlesShortWrites(t *testing.T) {
	w := &partialWriter{}
	data := bytes.Repeat([]byte("different data"), 100)
	if err := writeAll(w, data); err != nil || !bytes.Equal(w.Bytes(), data) {
		t.Fatal(err)
	}
	if err := writeAll(&partialWriter{zero: true}, data); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	if err := writeAll(&partialWriter{fail: true}, data); err == nil {
		t.Fatal("write failure hidden")
	}
}
func TestRealDataDeterministicDistinctAndBounded(t *testing.T) {
	create := func(seed uint64, job int) []byte {
		t.Helper()
		f, err := os.CreateTemp(t.TempDir(), "data")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err = prepareData(context.Background(), f, 2*dataChunk+4096, seed, job); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(f.Name())
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	first := create(42, 0)
	if len(first) != 2*dataChunk+4096 || !bytes.Equal(first, create(42, 0)) || bytes.Equal(first, create(42, 1)) || bytes.Equal(first, create(43, 0)) {
		t.Fatal("seed/job/full-size generation mismatch")
	}
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	w.Write(first)
	w.Close()
	if compressed.Len() < len(first)*95/100 {
		t.Fatal("preparation data is highly compressible")
	}
	f, err := os.CreateTemp(t.TempDir(), "cancel")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = prepareData(ctx, f, dataChunk, 42, 0); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
