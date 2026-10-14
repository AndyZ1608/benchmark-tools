# Illustrative benchmark summaries

These deterministic fixtures demonstrate the actual renderer. Every number is synthetic; no benchmark was run on this host. Paths below illustrate the report path that a real run would save. Repeated fixture values demonstrate formatting, not evidence about hardware.

## CPU (English, three repetitions)

```text

VMBENCH CPU TEST | Completed

Scaling efficiency (Integer)         90.0 % (4 Workers)
Scaling efficiency (Floating Point)  95.0 % (4 Workers)
CPU steal (whole run)                0.00 %
Reported integrity errors            0
Stability                            Stable

Assessment: Repeated measurements were consistent under the selected CV threshold.
Scaling is not absolute CPU speed; no comparable baseline is available for speed ranking.

Warnings:
  - Host contention and caches are uncontrolled; precision does not guarantee accuracy.

Elapsed: 12.00 s
Full report: results/cpu-fixture.json
```

## RAM (English, three repetitions)

```text

VMBENCH RAM TEST | Completed

TRIAD bandwidth            28.60 GB/s (4 Workers)
Dependent access latency   85.00 ns
Stability                  Stable
Reported integrity errors  0

Assessment: Repeated measurements were consistent under the selected CV threshold.
Application measurements include cache/loop effects; no comparable baseline is available for speed ranking.

Warnings:
  - Host contention and caches are uncontrolled; precision does not guarantee accuracy.

Elapsed: 12.00 s
Full report: results/ram-fixture.json
```

## Disk (English, three repetitions)

```text

VMBENCH DISK TEST | Completed

Workload                                Random Read 4K; QD32 x 1 jobs
IOPS                                    85,000 IOPS
Throughput                              332.03 MiB/s
p99 completion latency (job/run range)  1.240 ms
Stability                               Stable

Assessment: Repeated measurements were consistent under the selected CV threshold.
IOPS counts operations/second; p99 ranges retain each job/run distribution, and speed ranking needs a comparable baseline.

Warnings:
  - Host contention and caches are uncontrolled; precision does not guarantee accuracy.

Elapsed: 12.00 s
Full report: results/disk-fixture.json
```

## CPU (Vietnamese default, one repetition, unavailable steal telemetry)

```text

VMBENCH CPU TEST | Hoàn thành

Hiệu suất đa luồng (Integer)         90.0 % (4 Luồng)
Hiệu suất đa luồng (Floating Point)  95.0 % (4 Luồng)
CPU bị tranh chấp (toàn lần chạy)    Chưa đo
Số lỗi dữ liệu được báo cáo          0
Độ ổn định                           Chưa đủ dữ liệu

Nhận xét: Chưa đủ lần đo để đánh giá độ ổn định (cần ít nhất 3 lần).
Hiệu suất đa luồng không phải tốc độ CPU tuyệt đối; chưa có dữ liệu đối chiếu để xếp hạng tốc độ.

Lưu ý:
  - Tranh chấp tài nguyên và bộ nhớ đệm chưa được kiểm soát; precision không bảo đảm độ chính xác.
  - Đo nhanh chỉ mang tính sơ bộ; cần đo lặp lại để đối chiếu.

Thời gian đã chạy: 12.00 s
Báo cáo đầy đủ: results/cpu-quick-fixture.json
```
