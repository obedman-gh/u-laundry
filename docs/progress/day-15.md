# Laporan Hari 15 — Shift Kasir dan Rekonsiliasi Harian

## Tindakan — Hasil

- Shift kasir — buka kas, saldo awal, cash in/out beralasan, tutup kas, saldo sistem/fisik, selisih, catatan dan user penutup tersedia melalui service/API dan `/shifts.html`. Satu shift aktif per pencatat ditegakkan UNIQUE index. Saldo awal/penutupan tidak menjadi pendapatan.
- Izin — semua staf aktif memutasi shift sendiri; owner/admin membaca seluruh staf, cashier hanya miliknya. Body/query tidak dapat memilih actor lain. Replay tetap memerlukan izin terkini; shift tertutup tidak dapat diedit/reopen atau di-force-close oleh staf lain.
- Pembayaran — payment/refund tunai baru wajib shift aktif pencatat; non-tunai terkait shift bila tersedia dan tetap masuk rekonsiliasi tanpa shift bila tidak tersedia. Refund pada sesi berikutnya tidak menulis ulang hasil close lama. Posting/mutasi/close diserialisasi, diaudit dan idempotent; saldo negatif dan overflow ditolak.
- Laporan — periode transaksi harian WITA, shift semalam, penerimaan/refund per metode, cash in/out dan metode belum dicatat ditampilkan terpisah. Nominal mobile dapat dibaca melalui tabel geser; ringkasan dua shift menghasilkan satu halaman PDF A4 landscape. Tema biru existing dan frontend tanpa framework dipertahankan.
- Skema — migration 000011 menambah cash_shifts, shift_cash_movements dan ledger.shift_id/guard tanpa mengubah SQL 1–10. Upgrade tidak membuat shift atau mengisi relasi historis fiktif. Dokumen finance/business-rules/API/database/architecture/README/backlog diperbarui; tahap berikutnya Hari 16, dashboard dan laporan.

## Verifikasi

- `go test ./internal/domain ./internal/service -skip '^TestPostgres' -count=1 -timeout=60s` — lulus, termasuk rekonsiliasi/overflow, selisih positif-negatif, validasi close dan permission matrix. Unit HTTP/migration juga lulus pada pemeriksaan package terdampak; tidak menjalankan full suite.
- `go test ./internal/service -run '^TestPostgresCashShifts$' -count=1 -timeout=120s -v` — sembilan skenario lulus: delapan request buka identik hanya satu efek, duplicate active, cash/noncash association, mutasi/refund/reconciliation, ownership/replay, refund lintas shift, cash shortage/concurrency, close versus payment, rollback audit, constraint dan WITA/overnight. Metode historis tanpa klasifikasi juga teruji tanpa memengaruhi laci.
- `go test ./internal/migrate -run '^TestPostgresShiftUpgrade$' -count=1 -timeout=60s -v` — upgrade 10→11 dua kali lulus; histori/nominal/metode tetap, shift_id historis NULL dan tidak ada shift buatan.
- Targeted PostgreSQL `TestPostgresMigrations`, `TestPostgresRepositories`, `TestPostgresFinanceUpgrade`, `TestPostgresOrderWorkflowUpgrade`, `TestPostgresFinancePaymentsAndRefunds`, `TestPostgresPaymentUseCaseAtomicity`, `TestPostgresOrderConcurrency`, katalog/demo dan privasi — lulus pada schema 11. Fixture lama mendapat shift test eksplisit untuk cash; angka bisnis asli tetap. Hanya package/alur terdampak yang diperiksa.
- `go test ./internal/http/... ./internal/app -count=1 -timeout=60s` — lulus; routing/auth/CSRF, key/payload invalid, ownership denial, response null/array dan history. Tambahan bucket metode historis lulus melalui `go test ./internal/http/handler -run '^TestShiftHTTP' -count=1 -timeout=45s`.
- Browser Edge headless dengan API, cookie/CSRF dan PostgreSQL nyata terisolasi — login, buka, cash out, pembayaran cash/QRIS, close dengan selisih, akses cashier/owner dan DOM escaping lulus. Simulasi respons hilang setelah commit lalu retry isian sama menghasilkan satu mutasi. Pemeriksaan desktop 1280 px, mobile emulation 390 px, kolom geser/nominal dan PDF print lulus tanpa JavaScript exception; formulir/kontrol tidak ikut tercetak.
- `go vet ./internal/domain ./internal/service ./internal/repository/... ./internal/http/... ./internal/migrate`, `go build ./cmd/server ./cmd/migrate`, `node --check frontend/shifts.js` — lulus. gofmt, whitespace, UTF-8, tautan lokal dan diff/status diperiksa.
- PostgreSQL 16.14 test loopback 55448 — database disposable termasuk browser harness dibersihkan (0 tersisa), server/browser preview dan cluster test dihentikan. Tidak membaca `.env` atau mengubah database operasional. Harness/binary/log/screenshot/PDF sementara berada di cache luar repository; source harness sementara dihapus.
- `go test -race ./internal/service -run '^TestPostgresCashShifts$' -count=1 -timeout=120s`, CGO_ENABLED=1 — gagal build karena gcc tidak tersedia; race tidak diklaim lulus. Bukti concurrency di atas memakai PostgreSQL tanpa race instrumentation.
- Chime — chime-1.mp3 diputar dua kali melalui Windows MediaPlayer.

## Perubahan Penting

- internal/domain/shift.go, internal/service/shifts.go dan internal/repository/postgres/shifts.go — aturan/otorisasi, query, lock, replay, mutasi dan snapshot penutupan; tests unit/integration terarah.
- migrations/000011_cashier_shifts.up.sql — satu sesi aktif, immutable close, relasi/receipt ledger dan guard transaksi kas; bukti upgrade di internal/migrate/shifts_integration_test.go.
- internal/service/payments.go/refunds.go serta repository finansial — lock shift sebelum timestamp, cash guard dan shift_id pada projection receipt tanpa mengubah nominal/status laundry.
- internal/http/dto/shifts.go, handler/shifts.go, router dan cmd/server — tujuh endpoint privat, validasi dan dependency wiring.
- frontend/shifts.html/css/js dan navigasi antrean — pengelolaan kas sendiri, ringkasan/entries, retry key, offline/error states dan cetak; tanpa localStorage token atau antrean offline.
- [finance.md](../finance.md), [business-rules.md](../business-rules.md), API/database/architecture, README dan backlog — kebijakan, command, privilege serta upgrade/rollback schema 11.

## Risiko atau Sisa Pekerjaan

- Race belum terverifikasi karena toolchain C tidak tersedia. Android/printer fisik belum diuji; bukti tampilan memakai emulasi browser dan PDF.
- Database operasional belum dimigrasikan. Upgrade nyata wajib backup/restore salinan, rekonsiliasi dan satu writer schema 11; binary lama tanpa shift tidak boleh digunakan untuk posting tunai. Aplikasi belum dinyatakan layak produksi.
- Model kas per staf, tanpa laci bersama/force-close/transfer otomatis atau settlement bank/QRIS otomatis. Saldo awal dan fisik harus dihitung nyata; prosedur ini perlu ditinjau pemilik sebelum pemakaian operasional. Tidak ada blocker implementasi Hari 15 yang diketahui.
