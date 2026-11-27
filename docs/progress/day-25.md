# Laporan Hari 25 — Observability dan Operasi Dasar

## Tindakan — Hasil

- Structured logging — logger JSON slog mengikuti LOG_LEVEL; request mencatat request ID buatan server, route template, method, status, duration_ms, error_class dan response_aborted. Label disiapkan sebelum CORS/auth/limit agar penolakan tetap berkorelasi tanpa URL/token/query/header/body/PII. Method tak dikenal menjadi OTHER; error driver/panic/transport mentah tidak dicetak.
- Recovery — panic sebelum respons menghasilkan 500 JSON generik dengan ID dan membersihkan header sementara; setelah respons/flush dimulai koneksi di-abort tanpa menulis respons kedua. Status yang sudah dikirim tetap dicatat dan error count bertambah; kegagalan tulis dibedakan sebagai response_write.
- Health — `/api/health/live` tidak menyentuh DB; `/api/health/ready` memakai PingContext ber-timeout dan memeriksa draining sebelum/sesudah ping. `/api/health` tetap alias kompatibel. HEALTH_READINESS_TIMEOUT default 2s, positif dan lebih pendek dari HTTP_WRITE_TIMEOUT.
- Metrics — snapshot JSON request count/duration/error, pool DB dan tiga counter order/payment/refund non-sensitive tersedia pada `/api/ops/metrics`, hanya owner aktif. Denial memakai audit/permission manage_configuration existing. Registry concurrency-safe, maksimal 512 series, tanpa dependency/exporter/background worker baru. Replay/gagal tidak menambah business counter; counter per proses bukan ledger.
- Lifecycle — ReadHeaderTimeout/read/write/idle/shutdown existing dipertahankan dan diuji. Readiness menolak saat draining; shutdown normal menguras request sebelum DB close, timeout memaksa HTTP Close, dan kegagalan listener menutup koneksi aktif. Database ditutup tepat sekali pada jalur lifecycle teruji; error cleanup dipropagasikan dan dicatat tanpa detail sensitif.
- Operasi — docs/runbook.md memuat health, log/korelasi, metrics/pool, DB unavailable, migration failure, high error rate/panic, shutdown dan rollback. README, API, arsitektur, keamanan dan .env.example diselaraskan. Tidak mengubah DB/schema/dependency/frontend/.env operasional; tidak melakukan commit/push/deployment publik.

## Verifikasi

- `go test ./internal/telemetry ./internal/config ./internal/http/... ./internal/app -skip '^TestPostgres' -count=1 -timeout=60s` — lulus. Bukti mencakup level/redaksi JSON, correlation ID, route/method labels, panic sebelum/sesudah respons/flush, abort/write errors, health/deadline/draining, owner-only metrics, counter replay/failure dan batas registry.
- `go test ./internal/service -run '^TestOperationsAuthorizationUsesCurrentActiveOwner$' -count=1 -timeout=45s` — lulus: owner aktif diizinkan; admin/kasir, owner nonaktif dan actor hilang ditolak dengan denial audit pada fake repository.
- HTTP loopback dan fake DB — panic 500 diikuti request sehat, oversized header, shutdown drain/timeout, readiness saat draining tanpa ping, close tepat sekali serta propagasi/redaksi listener/close errors diuji. Registry juga diuji dengan 40 goroutine untuk mencegah lost updates; ini bukan pengganti race detector atau performance test.
- `go vet ./internal/telemetry ./internal/config ./internal/http/... ./internal/app ./internal/service ./cmd/server` dan build cmd/server ke cache workspace — lulus. Format, diff/status dan dokumen diperiksa; tidak menambahkan dependency.
- Tidak menjalankan full UI test, full suite, PostgreSQL operasional, migration nyata atau pengujian server publik. Race detector belum dijalankan karena compiler C pendukung tidak tersedia pada sesi Windows ini.

## Perubahan Penting

- `internal/telemetry`, middleware request/router — JSON logger, klasifikasi, recovery, route labels dan bounded process metrics.
- `internal/http/handler/operations.go`, `internal/service/operations.go` — liveness/readiness dan authorization snapshot metrics; handler order/payment/refund mengukur hasil non-replay.
- `internal/app`, `cmd/server`, `internal/config`, `.env.example` — wiring logger/pool, timeout readiness, drain/cleanup dan transport error redaction.
- `docs/runbook.md`, README, API, arsitektur, keamanan — kontrak observability, prosedur gangguan/rollback dan batas bukti; targeted tests baru mengikuti command resmi README.

## Risiko atau Sisa Pekerjaan

- Readiness hanya memeriksa ping dependency, bukan schema/izin setiap tabel atau keberhasilan transaksi. Metrics juga memerlukan DB untuk sesi/authorization, sehingga tidak selalu dapat diakses saat DB down.
- Counter/log hanya proses saat ini, tanpa agregasi antar-instance atau retensi otomatis. Counter bisnis dapat hilang antara commit dan increment/restart; rekonsiliasi tetap memakai ledger/audit. Error response atau koneksi terputus tidak membuktikan transaksi belum commit.
- Handler/driver harus menghormati context; force close tidak menunggu goroutine yang mengabaikannya. Parser/transport error sebelum middleware tidak selalu memiliki request ID. Log proxy/Cloudflare tetap perlu redaksi sendiri.
- Race detector, DB outage nyata, deployment Ubuntu/systemd/proxy/Tunnel, monitoring/retensi supervisor serta backup/restore/rollback drill produksi belum diverifikasi. Tidak ada blocker implementasi yang diketahui untuk scope Hari 25; kesiapan produksi menyeluruh tidak diklaim.
