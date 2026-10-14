package presentation

import "strings"

var words = map[string][2]string{
	"completed": {"Completed", "Hoàn thành"}, "failed": {"Failed", "Thất bại"}, "skipped": {"Skipped", "Bỏ qua"}, "not_run": {"Not run", "Chưa chạy"}, "unknown": {"Insufficient data", "Chưa đủ dữ liệu"}, "running": {"In progress", "Đang thực hiện"},
	"stable": {"Stable", "Ổn định"}, "variable": {"Variable", "Biến động"}, "insufficient": {"Insufficient data", "Chưa đủ dữ liệu"}, "investigate": {"Needs investigation", "Cần kiểm tra"},
	"status": {"Status", "Trạng thái"}, "assessment": {"Assessment", "Nhận xét"}, "stability": {"Stability", "Độ ổn định"}, "report": {"Full report", "Báo cáo đầy đủ"}, "warnings": {"Warnings", "Lưu ý"}, "errors": {"Errors", "Lỗi"}, "elapsed": {"Elapsed", "Thời gian đã chạy"},
	"unavailable": {"Not measured", "Chưa đo"}, "baseline": {"No comparable baseline is available for speed ranking.", "Chưa có dữ liệu đối chiếu tương đương để xếp hạng tốc độ."},
	"limited":    {"Insufficient repetitions to evaluate consistency (at least 3 required).", "Chưa đủ lần đo để đánh giá độ ổn định (cần ít nhất 3 lần)."},
	"consistent": {"Repeated measurements were consistent under the selected CV threshold.", "Các lần đo nhất quán theo ngưỡng CV đã chọn."}, "varied": {"Performance varied between repetitions; review the detailed report.", "Kết quả thay đổi giữa các lần đo; hãy xem báo cáo chi tiết."},
	"integrity": {"Reported integrity errors", "Số lỗi dữ liệu được báo cáo"}, "integer_scaling": {"Scaling efficiency (Integer)", "Hiệu suất đa luồng (Integer)"}, "float_scaling": {"Scaling efficiency (Floating Point)", "Hiệu suất đa luồng (Floating Point)"},
	"steal": {"CPU steal (whole run)", "CPU bị tranh chấp (toàn lần chạy)"}, "busy": {"CPU busy (guest average)", "CPU hoạt động (trung bình VM)"}, "steal_note": {"CPU steal time was detected: the guest waited for host CPU time.", "Có CPU steal: VM phải chờ tài nguyên CPU từ máy chủ."},
	"cpu_note":    {"Scaling describes use of additional threads, not absolute CPU speed.", "Hiệu suất đa luồng cho biết mức tận dụng thêm luồng, không phải tốc độ CPU tuyệt đối."},
	"single_only": {"Single-thread measurements only; multi-thread scaling was not measured.", "Chỉ có kết quả một luồng; chưa đo khả năng mở rộng đa luồng."},
	"stress_note": {"High CPU busy time is expected during stress. Completion does not prove hardware reliability.", "CPU hoạt động cao là bình thường khi stress. Hoàn thành không chứng minh phần cứng đáng tin cậy."},
	"stress_done": {"The stress workload finished; review any reported failures separately.", "Tác vụ stress đã kết thúc; cần xem riêng các lỗi được báo cáo."},
	"bandwidth":   {"TRIAD bandwidth", "Băng thông TRIAD"}, "latency": {"Dependent access latency", "Độ trễ truy cập phụ thuộc"}, "ram_note": {"Application-level bandwidth and dependent-access latency were measured; cache and loop overhead can affect them.", "Đây là băng thông ứng dụng và độ trễ truy cập phụ thuộc; bộ nhớ đệm và vòng lặp có thể ảnh hưởng kết quả."},
	"cache_unknown": {"LLC size is unavailable; main-memory bandwidth cannot be confirmed.", "Chưa biết kích thước LLC; chưa thể xác nhận băng thông RAM ngoài bộ nhớ đệm."},
	"cache_small":   {"The per-worker array is smaller than 4x the reported LLC; cache effects may dominate TRIAD throughput.", "Mảng mỗi luồng nhỏ hơn 4 lần LLC được báo cáo; bộ nhớ đệm có thể chi phối tốc độ TRIAD."},
	"workload":      {"Workload", "Kiểu đo"}, "queue": {"Configured queue depth / jobs", "Độ sâu hàng đợi / số job"}, "iops": {"IOPS", "Thao tác I/O mỗi giây"}, "throughput": {"Throughput", "Tốc độ truyền dữ liệu"}, "p99": {"p99 completion latency (job/run range)", "Độ trễ hoàn tất p99 (khoảng theo job/lần đo)"},
	"read": {"Read", "Đọc"}, "write": {"Write", "Ghi"}, "disk_note": {"IOPS counts completed operations per second; backend storage health cannot be inferred from this test.", "IOPS là số thao tác hoàn tất mỗi giây; phép đo này không kết luận sức khỏe hệ thống lưu trữ."},
	"percentile_note": {"Latency ranges preserve individual job/run percentiles; they are not an overall percentile.", "Khoảng độ trễ giữ nguyên phân vị từng job/lần đo, không phải phân vị tổng hợp."},
	"protocol":        {"Protocol", "Giao thức"}, "direction": {"Direction", "Chiều truyền"}, "duration": {"Measured duration", "Thời gian đo"}, "loss": {"UDP packet loss (per-run range)", "Mất gói UDP (khoảng theo lần đo)"}, "jitter": {"UDP jitter (per-run range)", "Jitter UDP (khoảng theo lần đo)"},
	"network_note": {"This is endpoint throughput for the selected configuration, not maximum packet-forwarding capacity.", "Đây là tốc độ giữa hai đầu cuối theo cấu hình đã chọn, không phải năng lực chuyển tiếp gói tối đa."},
	"rtt_note":     {"RTT was not measured.", "Chưa đo RTT."}, "peer_missing": {"No test peer configured.", "Chưa cấu hình máy đối tác để đo mạng."},
	"failure_note": {"The benchmark failed; partial observations do not represent a completed test.", "Phép đo thất bại; dữ liệu một phần không đại diện cho phép đo hoàn chỉnh."},
	"notrun_note":  {"This module was not run after an earlier failure.", "Phần này chưa chạy do lỗi ở phần trước."}, "missing_note": {"No validated measurements are available.", "Chưa có số đo hợp lệ."},
	"more_warnings": {"Additional warnings are in the full report.", "Các lưu ý khác có trong báo cáo đầy đủ."}, "no_report": {"Report could not be saved; see the error below.", "Không thể lưu báo cáo; xem lỗi bên dưới."},
	"full": {"FULL BENCHMARK RESULTS", "KẾT QUẢ ĐO TỔNG HỢP"}, "details": {"Details", "Chi tiết"}, "count": {"Runs", "Lần đo"}, "workers": {"Workers", "Luồng"}, "median": {"Median", "Trung vị"}, "min": {"Min", "Nhỏ nhất"}, "max": {"Max", "Lớn nhất"}, "cv": {"CV", "CV"},
	"preparing": {"Preparing workload", "Chuẩn bị tác vụ đo"}, "phase_done": {"Phase completed", "Đã hoàn tất giai đoạn"},
}

func text(lang, key string) string {
	if v, ok := words[key]; ok {
		if lang == "en" {
			return v[0]
		}
		return v[1]
	}
	return key
}
func tr(lang, en, vi string) string {
	if lang == "en" {
		return en
	}
	return vi
}
func warningText(lang, s string) string {
	if strings.HasPrefix(s, "Highest observed job/run p99 increased: ") {
		return tr(lang, "Highest observed job/run p99 increased: ", "Giá trị p99 cao nhất theo job/lần đo tăng: ") + strings.TrimPrefix(s, "Highest observed job/run p99 increased: ")
	}
	switch s {
	case "Preliminary workload; unsuitable for rigorous cross-platform comparisons":
		return tr(lang, "Quick test is preliminary; use repeated measurements for comparisons.", "Đo nhanh chỉ mang tính sơ bộ; cần đo lặp lại để đối chiếu.")
	case "Virtualization, backend contention and cache effects are uncontrolled; precision does not guarantee absolute accuracy":
		return tr(lang, "Host contention and caches are uncontrolled; precision does not guarantee accuracy.", "Tranh chấp tài nguyên và bộ nhớ đệm chưa được kiểm soát; precision không bảo đảm độ chính xác.")
	case "Disk target is on the system filesystem":
		return tr(lang, "The disk test targets the system filesystem.", "Phép đo đĩa chạy trên hệ thống tệp của hệ điều hành.")
	case "Read tests also write dataset initialization. Direct I/O does not bypass all storage caches. Benchmark files limited to selected size.":
		return tr(lang, "Read tests write preparation files; direct I/O does not bypass every storage cache.", "Đo đọc vẫn ghi tệp chuẩn bị; direct I/O không bỏ qua mọi bộ nhớ đệm lưu trữ.")
	case "Network skipped: no peer configured":
		return text(lang, "peer_missing")
	case "Fewer than three repetitions; limited stability evidence":
		return text(lang, "limited")
	}
	for _, pair := range words {
		if s == pair[0] || s == pair[1] {
			return tr(lang, pair[0], pair[1])
		}
	}
	return s
}
