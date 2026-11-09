# Laporan Hari 11 — Pelanggan dan Privasi Tracking

## Tindakan — Hasil

- Customer management — nama wajib, kontak/alamat/catatan opsional; API dan halaman customers.html menyediakan create/edit/search, pagination dan riwayat order. Edit memakai lock/version serta audit atomik; profil dapat mengosongkan data opsional tanpa mengubah snapshot order lama.
- Tracking existing — generator crypto/rand 256 bit dan penyimpanan SHA-256 dipertahankan; tidak ada fallback dari ID order. Respons publik kini tepat tracking_code/status/estimated_ready_at/message, tanpa identitas pelanggan, internal order ID, milestone tambahan, layanan, harga, pembayaran atau ledger.
- Kebijakan capability — expiry 90 hari dan rotasi privat oleh staf ditambahkan; lock/version menolak rotasi stale/bersamaan, token lama dicabut setelah commit. UNIQUE hash penuh dan CHECK panjang 32 byte tetap; collision/audit failure rollback tanpa token sukses. Raw token hanya pada create/rotate; UI membagikan fragment dan tidak menyimpan token di browser storage.
- Receipt — tetap internal dengan cookie dan izin use case; HTML escaped dan no-store. Public receipt tidak ditambahkan karena belum ada kebutuhan bisnisnya.
- Migration 000007 — menambah address/version customer, expires_at finite token dan aksi audit rotasi; token existing mendapat grace 90 hari sejak migration. Tidak ada merge/delete/import, modifikasi migration 000001–000006, atau penerapan ke database operasional.

## Verifikasi

- `go test ./internal/domain ./internal/platform ./internal/service ./internal/http/... ./internal/migrate -run 'TestTracking|TestCustomer|TestCreateOrder|TestServiceFailuresAreRedactedAndReceiptIsEscaped|TestExistingRoutes|TestCatalog$' -count=1 -timeout=60s` — lulus: canonical token, data minimum/edit/search, authorization/rollback, empat field publik exact, invalid/missing token, proteksi receipt GET/HEAD, CSRF/rotation dan boundary customer/history. Sampel 4.096 token 256 bit tanpa duplikat; keamanan tetap berasal dari crypto/rand, bukan sampling.
- PostgreSQL 16: `go test ./internal/service -run '^TestPostgresCustomerPrivacy$' -count=1 -timeout=90s -v` dan `go test ./internal/repository/postgres -run '^TestPostgresTrackingRepositoryConstraints$' -count=1 -timeout=60s -v` — lulus pada cluster test baru loopback 55441 dengan ULAUNDRY_TEST_ADMIN_DSN, tanpa membaca .env. Bukti upgrade 6→7 dua kali/history terjaga/grace token existing, search literal/nomor lokal, snapshot customer, version, constraint hash/uniqueness/finite expiry, batas expiry eksak, collision rollback, audit rollback dan dua rotasi bersamaan (1 sukses, 1 konflik); token lama ditolak.
- `node --test scripts/frontend-contract.test.cjs` — 11/11 lulus: alur login/staf existing serta safe DOM customer, name-only create, history/receipt link, rotasi dengan CSRF/version, stale edit mengosongkan pilihan/PII dan tracking fragment tanpa storage.
- `go build ./...` dan `go vet ./internal/domain ./internal/service ./internal/repository/postgres ./internal/http/...` — lulus. gofmt, diff/status, encoding UTF-8 dan tautan dokumen diperiksa. Cache Go diarahkan ke .cache workspace karena cache profil Windows tidak dapat ditulis. Tidak menjalankan full suite atau integration finansial umum.
- Cleanup PostgreSQL — 0 database disposable tersisa; cluster test dihentikan. Tidak memakai database operasional atau mengubah .env.

## Perubahan Penting

- internal/domain, service, repository/postgres, HTTP DTO/handler/router dan composition root — customer workflow, projection publik, expiry dan rotasi transaksional; SQL tetap di repository.
- migrations/000007_customer_privacy.up.sql — schema tambahan tanpa mengubah histori finansial atau hash token existing; rollback memakai forward fix, bukan downgrade penulis schema 6.
- frontend/customers.html/customers.js/customers.css dan tracking.html/tracking.js — UI sederhana dengan warna existing, data minimum, label/focus, loading/empty/error dan navigasi antrean.
- docs/privacy.md, docs/api.md, README, architecture/database dan laporan ini — kontrak, kebijakan token, retensi, hak runtime, risiko URL/log dan command verifikasi terarah.

## Risiko atau Sisa Pekerjaan

- Migration/deployment operasional belum dijalankan; writer schema 6 tidak kompatibel karena expiry kini wajib. Backup/restore salinan dan deployment terkontrol tetap diperlukan. Aplikasi belum dinyatakan siap produksi menyeluruh.
- API GET tracking tetap membawa token di path; proxy/Cloudflare/analytics produksi wajib meredaksi atau mengecualikannya dan bypass cache API. Konfigurasi infrastruktur tersebut belum diverifikasi. Tautan tetap capability rahasia bagi penerimanya.
- UI diverifikasi dengan DOM sintetis; browser/mobile visual, printer dan perilaku perangkat bersama belum diuji. Retensi/purge profil, snapshot order, audit dan backup belum diotomatisasi; kebijakan perlu ditetapkan owner.
