# Laporan Hari 14 — Pembayaran, Refund dan Buku Kas

## Tindakan — Hasil

- Pemeriksaan pekerjaan tertunda — laporan terakhir Hari 13, commit 689bc92, dengan implementasi Hari 14 belum selesai di working tree. Perubahan existing dipertahankan; penyelesaian dibatasi pada payment/refund/ledger, tanpa commit, push atau deployment.
- Pembayaran dan refund — implementasi parsial diverifikasi dan dituntaskan: DP/metode campuran, overpayment ditolak, refund penuh satu receipt oleh owner/admin sebelum pengambilan, history privat dan saldo net konsisten. Delapan request identik menghasilkan satu pembayaran dan satu refund, tanpa posting ulang. Refund adalah pencatatan manual, tanpa gateway/pemindahan uang otomatis.
- Integritas — payment/refund/ledger append-only, unique reference dan posting wajib cocok saat commit. Bukti tambahan menolak metode/actor/waktu posting berbeda, mempertahankan laporan/daftar setelah refund serta menolak replay setelah izin dicabut. Kegagalan ledger/audit membatalkan transaksi finansial.
- Upgrade — empat skenario baru membuktikan upgrade 9→10 berulang mempertahankan nominal/history/order, backfill metode cash/QRIS tanpa mengarang refund dan penolakan atomik missing posting, waktu posting berbeda serta key invalid. SQL migration 1–9 tidak diubah.
- Dokumentasi — aturan finansial, API, hak runtime, compatibility/rollback dan backlog diperbarui per 24 November 2026. Tahap berikutnya Hari 15, shift dan rekonsiliasi; aplikasi belum layak produksi.

## Verifikasi

- `go test ./internal/domain ./internal/service ./internal/http/... ./internal/migrate -skip '^TestPostgres' -count=1 -timeout=60s` — lulus. Tambahan test admin/refund/replay revoked lulus melalui `go test ./internal/service -run '^TestRefundUseCaseReplayAuthorizationAndRollback$' -count=1 -timeout=45s`.
- `go test ./internal/service -run '^TestPostgres(FinancePaymentsAndRefunds|PaymentUseCaseAtomicity|OrderConcurrency)$' -count=1 -timeout=120s -v` — lulus: tujuh skenario finansial, atomicity payment dan sepuluh skenario concurrency order. Setelah tambahan pemeriksaan laporan/saldo dan enam mismatch posting, `TestPostgresFinancePaymentsAndRefunds` dijalankan ulang terarah dan lulus.
- `go test ./internal/migrate ./internal/repository/postgres -run '^TestPostgres(Migrations|Repositories|OrderWorkflowUpgrade)$' -count=1 -timeout=120s -v` — lulus: fresh/upgrade/repeat/constraint, repository dan kompatibilitas workflow. `go test ./internal/migrate -run '^TestPostgresFinanceUpgrade$' -count=1 -timeout=90s -v` — empat skenario baru lulus.
- `go test ./internal/service -run '^TestPostgres(CatalogHistoryAndOrderSnapshots|CatalogDemo|CustomerPrivacy)' -count=1 -timeout=90s -v` — lulus untuk fixture/guard seed yang mengikuti schema 10.
- `go vet ./internal/domain ./internal/service ./internal/repository/... ./internal/http/... ./internal/migrate` dan `go build ./cmd/server ./cmd/migrate` — lulus; gofmt bersih. Git diff/status, whitespace dan tautan dokumen diperiksa.
- PostgreSQL 16.14 — cluster baru loopback 55447, DSN test eksplisit, database disposable tanpa membaca `.env` atau database operasional. Cleanup memverifikasi 0 database test tersisa; cluster dihentikan.
- `go test -race ./internal/service -run '^TestRefundUseCaseReplayAuthorizationAndRollback$' -count=1 -timeout=60s`, CGO_ENABLED=1 — gagal build karena gcc tidak tersedia. Race tidak diklaim lulus.
- Chime — chime-1.mp3 diputar dua kali melalui Windows MediaPlayer; pemutaran selesai tanpa error. Backend audio MCI tidak tersedia, sehingga digunakan WPF.

## Perubahan Penting

- internal/domain, internal/service/payments.go dan refunds.go, internal/repository/postgres — status receipt/refund, saldo net dan transaksi payment/refund/ledger/audit dari pekerjaan parsial Hari 14 dipertahankan serta diverifikasi.
- migrations/000010_payment_refunds.up.sql dan internal/migrate/finance_integration_test.go — skema finansial, bukti upgrade historis dan kegagalan atomik.
- internal/service/finance_integration_test.go dan refunds_test.go — concurrency, rollback, laporan/summary net, mismatch posting serta otorisasi replay terkini.
- internal/http/handler/finance.go, DTO/router dan tests — respons receipt lengkap, endpoint refund/history privat dan boundary validation.
- [finance.md](../finance.md), API, database, business-rules, architecture, README dan backlog — aturan/batas finansial, command verifikasi dan tahap berikutnya.

## Risiko atau Sisa Pekerjaan

- Race belum terverifikasi karena compiler C tidak tersedia. Bukti concurrency PostgreSQL lulus tanpa race instrumentation.
- Refund parsial/setelah pengambilan, shift, UI pembayaran/refund dan penerimaan printer/Android belum tersedia atau belum diverifikasi; dilanjutkan sesuai backlog Hari 15–30.
- Database operasional, backup/restore nyata dan cutover/deployment belum dilakukan. Upgrade nyata wajib backup/restore salinan, rekonsiliasi dan satu writer; binary sebelum schema 10 tidak boleh dipakai setelah refund. Kebijakan refund perlu ditinjau pemilik sebelum aktivasi operasional.
