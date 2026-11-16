# Laporan Hari 13 — Workflow, Concurrency dan Idempotensi Order

## Tindakan — Hasil

- Pemeriksaan pekerjaan tertunda — laporan terakhir Hari 12 dan repository awal bersih pada fd0939a; implementasi awal Hari 13 sudah tersimpan tetapi belum dilaporkan. Penyelesaian dibatasi pada workflow Hari 13; backlog kini mencatat Hari 01–13 dan tahap berikutnya Hari 14.
- Workflow — create/status/cancel memakai receipt idempotensi actor/key, lock order dan expected version; milestone processing, histori status serta audit atomik dipertahankan. Sepuluh skenario concurrency PostgreSQL membuktikan satu efek untuk request identik, konflik payload/stale, transisi invalid, rollback, payment versus cancel/pickup dan satu posting ledger.
- Kontrak HTTP — replay create kini menandai replayed secara konsisten pada envelope dan order; 18 kasus header create/status/cancel, replay tanpa capability, riwayat/pagination dan proteksi route diuji. Payload/key semula mengembalikan keadaan terkini dan applied_version tanpa memundurkan order.
- Upgrade — migration 000009 existing tidak diubah; bukti baru upgrade 8→9 dua kali mempertahankan lima status historis, nominal, versi dan milestone. Baseline tidak mengarang actor/transisi; processing_at lama tetap NULL. Fixture versi masa depan, fixture korupsi snapshot dan guard seed legacy diselaraskan ke schema 9.
- Status — ulaundry siap dilanjutkan ke tahap selanjutnya: Hari 14, payment/ledger dan aturan refund. Ini bukan pernyataan siap produksi.

## Verifikasi

- `go test ./internal/domain ./internal/service ./internal/http/... ./internal/migrate -skip '^TestPostgres' -count=1 -timeout=60s` — lulus; domain, rollback/replay/otorisasi use case, kontrak HTTP dan routing tanpa DB operasional atau .env.
- PostgreSQL `TestPostgresOrderConcurrency`, `TestPostgresOrderWorkflowUpgrade`, `TestPostgresCatalogHistoryAndOrderSnapshots`, `TestPostgresCatalogDemo`, `TestPostgresPaymentUseCaseAtomicity` — lulus dengan `go test` terarah, count=1/deadline eksplisit; 10 skenario concurrency, snapshot harga, demo, rollback finansial dan upgrade 8→9.
- PostgreSQL `TestPostgresMigrations` dan `TestPostgresRepositories` — skenario fresh/upgrade/constraint/locking/replay lulus. Tiga regresi seed/future version/fixture korupsi diperbaiki; hasil akhir kasus terkait lulus melalui `go test ./internal/migrate -run '^TestPostgresMigrations$/(demo_opt_in_empty_database_only|history_drift)$' -count=1 -timeout=60s -v`, `go test ./internal/migrate -run '^TestPostgresMigrations$/^history_drift#03$' -count=1 -timeout=60s -v` dan `go test ./internal/repository/postgres -run '^TestPostgresRepositories$/corrupt_persisted_snapshot_is_not_exposed$' -count=1 -timeout=60s -v`. Tidak mengklaim satu full-suite run bersih.
- `go vet ./internal/domain ./internal/service ./internal/repository/... ./internal/http/... ./internal/migrate` dan `go build ./cmd/server ./cmd/migrate` — lulus. gofmt bersih, git diff --check/status diperiksa; 7 dokumen valid UTF-8 dan 58 tautan file lokal valid.
- PostgreSQL 16.14 — cluster test baru loopback 55445, database disposable dari ULAUNDRY_TEST_ADMIN_DSN, tanpa membaca konfigurasi/database operasional. Cleanup memverifikasi 0 database test tersisa; cluster test dihentikan.
- Chime — sounds/chime-1.mp3 selesai diputar dua kali melalui audio Windows.
- Race — percobaan `go test -race ./internal/service -run '^TestOrderCommand|^TestOrderCreateCommand' -count=1 -timeout=60s` dengan CGO_ENABLED=1 gagal build karena compiler C gcc tidak tersedia. Tidak mengklaim race lulus; command race concurrency DB pada host yang sesuai tersedia di README.

## Perubahan Penting

- internal/http/handler/pesanan.go dan order_commands_test.go — respons replay konsisten dan bukti header/history; router_test.go mencakup autentikasi riwayat.
- internal/migrate/order_workflow_integration_test.go dan fixture integration repository/migration — upgrade lima status, baseline, chronology/version, snapshot corruption dan drift.
- internal/service/order_commands_test.go — kegagalan create/commit tidak membocorkan capability/receipt dan replay tetap memerlukan izin. File workflow/test parsial diformat tanpa perubahan kebijakan bisnis.
- scripts/demo.sql — guard schema legacy demo mengikuti versi 9, tanpa akun atau penggunaan data nyata.
- README, API, architecture, business-rules, database dan backlog — kontrak retry/conflict, milestone, privilege, upgrade/rollback serta tahap berikutnya diperbarui.

## Risiko atau Sisa Pekerjaan

- Race belum terverifikasi; jalankan test concurrency dengan race pada host yang memiliki toolchain C kompatibel. Bukti concurrency PostgreSQL telah lulus tanpa race instrumentation.
- Migration/deployment/cutover data operasional, UI browser/Android dan printer belum dilakukan. Writer sebelum schema 9 tidak boleh diaktifkan kembali tanpa compatibility review; client mutasi order wajib mengirim key.
- Hari 14–30 tetap terbuka: refund/payment lengkap, shift, laporan, UI POS, PWA, hardening, operasi/backup, CI dan acceptance rilis. Tidak ada blocker implementasi Hari 13 yang diketahui.
