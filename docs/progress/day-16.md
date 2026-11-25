# Laporan Hari 16 — Dashboard dan Laporan Operasional

## Tindakan — Hasil

- Pemeriksaan pekerjaan tertunda — Hari 15 sudah memiliki implementasi dan bukti; backlog dilanjutkan ke Hari 16. Perubahan lokal Hari 15 tetap dipertahankan, tanpa commit/push/deployment atau akses database operasional.
- Dashboard — tiga endpoint privat menyediakan order per periode, antrean, piutang, terlambat, sepuluh layanan populer, penerimaan/refund/metode pembayaran dan ringkasan shift. Snapshot saat ini dibedakan dari periode transaksi; WITA default, akhir eksklusif dan batas 366 hari.
- Keuangan — pembayaran/refund bersumber ledger; biaya, cash in/out, saldo awal dan snapshot penutupan dipisahkan. Fixture menghasilkan Rp21.000 penerimaan, Rp10.000 refund, Rp11.000 net, Rp28.000 piutang, tiga order terlambat dan selisih shift −Rp50 sesuai transaksi sintetis.
- Detail/CSV — detail order/ledger berhalaman 1–100 baris; CSV hanya halaman yang diminta, escaping delimiter serta proteksi formula/control character. Cookie sesi, izin owner/admin, no-store dan HEAD ditegakkan; query tidak dapat memilih actor lain.
- Konsistensi — read-only REPEATABLE READ mempertahankan snapshot dashboard ketika koneksi lain menambah pembayaran; audit commit dan pengecekan ulang staf diwajibkan sebelum mengembalikan hasil. Kegagalan/overflow menahan hasil seluruh laporan.
- Kompatibilitas — schema tetap 000011 dan dependency tetap. Tes autentikasi yang masih mengharapkan delapan migration diperbaiki mengikuti daftar migration embedded, lalu lulus pada schema 11.

## Verifikasi

- `go test ./internal/service -skip '^TestPostgres' -count=1 -timeout=60s` — lulus, termasuk unit laporan/izin dan kegagalan read/audit/commit/pencabutan staf serta validasi periode/filter.
- `go test ./internal/service -run '^TestPostgresOperationalReports$' -count=1 -timeout=90s -v` — enam subtest lulus: kosong, angka/batas waktu, pagination/filter, izin/audit rollback, snapshot dengan writer lain, context cancellation dan overflow. Data dibentuk melalui use case/repository dengan constraint nyata; periode awal/akhir, DP lintas tanggal, refund/cancel, diskon, metode tanpa shift, shift semalam dan estimasi tepat batas tercakup.
- `go test ./internal/http/... -skip '^TestPostgres' -count=1 -timeout=60s` — lulus; routing/auth boundary, DTO/error dan query invalid. `go test ./internal/http/handler -run '^TestReport' -count=1 -timeout=45s -v` — lulus setelah pemeriksaan akhir; CSV escaping/formula/whitespace/control, nilai integer, timestamp WITA dan HEAD.
- `go test ./internal/http -run '^TestPostgres(ReportHTTP|CookieAuthenticationLifecycle)$' -count=1 -timeout=90s -v` — kedua test lulus. HTTPS dengan login/cookie/sesi PostgreSQL nyata: anonymous 401, cashier 403, owner JSON/CSV, Rp4.000 terbayar/Rp6.000 piutang, privasi ledger dan HEAD. Test ReportHTTP diulang setelah pemetaan satuan publik item diperbaiki — lulus.
- `go build ./...` dan `go vet ./internal/service ./internal/repository/... ./internal/http/... ./internal/app` — lulus. gofmt seluruh file Go terdampak dan `git diff --check` — bersih; status/diff diperiksa, tanpa full suite/release checks.
- PostgreSQL 16.14 loopback 55448 pada cluster test existing — semua database disposable test dibersihkan, hitungan tersisa nol; cluster test dihentikan dan pg_ctl memastikan tidak ada server berjalan. Tidak membaca `.env`, mengubah schema/data operasional, atau menjalankan migration publik.
- Dokumentasi README/API/architecture/backlog/reports dan laporan ini — diperbarui; tautan lokal dan UTF-8 diperiksa.
- Chime — chime-1.mp3 selesai diputar dua kali melalui Windows MediaPlayer .NET, posisi akhir 2,376 detik pada masing-masing pemutaran.

## Perubahan Penting

- `internal/repository/reports.go` dan `postgres/reports.go` — projection/port, lima query dashboard tanpa N+1, detail berurutan stabil, aggregate BIGINT dan error context-aware.
- `internal/service/reports.go` — periode/otorisasi, snapshot baca, audit setelah read commit, serta hasil kosong saat gagal; unit dan integration tests terarah.
- `internal/http/dto/reports.go`, `handler/reports.go` dan router — `/api/laporan/dashboard`, `/api/laporan/orders`, `/api/laporan/ledger`, JSON/CSV dan metadata halaman; HTTPS test baru. Kontrak `/neraca` tetap.
- `internal/http/auth_integration_test.go` — ekspektasi history migration mengikuti daftar embedded, tanpa mengubah autentikasi aplikasi.
- [reports.md](../reports.md), [API](../api.md), [architecture](../architecture.md), README dan backlog — definisi angka, batas snapshot/CSV, perintah test dan tahap selanjutnya dicatat.

## Risiko atau Sisa Pekerjaan

- Hari 17 — audit trail/activity history — menjadi tahap berikutnya. UI dashboard/export/print Hari 22 masih tertunda; AC11 belum ditandai selesai dan belum ada verifikasi browser/perangkat/printer untuk UI laporan.
- Tiap halaman CSV mempunyai snapshot sendiri; export beberapa halaman ketika ada penulisan bukan snapshot massal beku. Angka antrean/piutang/terlambat adalah keadaan sekarang, bukan rekonstruksi historis. Laporan bukan double-entry, accrual accounting, settlement bank atau backup.
- Tidak ada indeks/migration baru atau EXPLAIN/benchmark kosmetik; ukur pada dataset operasional anonim bila ukuran/latensi membenarkan. Tidak ada blocker implementasi Hari 16 yang diketahui.
- Gate produksi sebelumnya tetap: race instrumentation, cutover/import data nyata, kebijakan pemilik, deployment, backup/restore dan acceptance UI/operasional belum dituntaskan. Aplikasi belum dinyatakan layak produksi.
