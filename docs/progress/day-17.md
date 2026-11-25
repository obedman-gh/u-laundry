# Laporan Hari 17 — Audit Trail dan Activity History

## Tindakan — Hasil

- Audit sensitif — akun/role/password, katalog/harga/konfigurasi, status order, pembayaran/refund dan shift close tetap atomik dengan perubahan bisnis. Korelasi request server serta metadata bertipe version/status/role/active/unit_price tersimpan; tanpa credential, token, secret atau snapshot PII.
- Login — credentials_verified dibedakan dari login_success yang atomik dengan registrasi sesi. Marker staff_logged_in historis tetap; sesi gagal tidak menghasilkan login_success/cookie palsu. Penolakan keamanan tetap tercatat terpisah setelah rollback bisnis.
- Append-only — migration 000012 menambah metadata/korelasi, indeks penelusuran dan penolakan TRUNCATE pada kedua tabel audit; UPDATE/DELETE tetap ditolak. SQL 1–11 immutable, request ID historis tidak dikarang dan tidak ada data nyata yang dihapus.
- Owner — `/audit.html` dan `/api/audit/events` menyediakan filter tanggal WITA/aksi/sumber/pelaku/objek/request ID, limit 1–100 dan pagination stabil. Endpoint objek lama kini owner-only juga; admin/cashier ditolak. Halaman mempertahankan tema biru dan safe DOM, termasuk keadaan loading/kosong/offline/gagal.
- Retensi — minimum bisnis 24 bulan, keamanan 90 hari, peninjauan triwulanan dan arsip privat terverifikasi ditetapkan. Tidak ada purge API, scheduler penghapusan atau perubahan database operasional.

## Verifikasi

- `go test ./internal/domain ./internal/service ./internal/http/... ./internal/migrate -skip '^TestPostgres' -count=1 -timeout=60s` — lulus setelah perubahan fondasi. Tambahan test audit/permission/HTTP dijalankan dengan `-run '^(TestAudit|TestPermissionMatrix|TestStaffHTTPPermissionMatrix)'` — lulus, termasuk owner/admin/cashier dengan signed claim owner tetapi role service terkini.
- `go test ./internal/migrate -run '^TestPostgresAuditUpgrade$' -count=1 -timeout=60s -v` — lulus; upgrade 11→12 dua kali, history/kolom lama tetap, request ID historis NULL, metadata liar/tipe invalid dan INSERT tanpa request ditolak; UPDATE/DELETE/TRUNCATE kedua tabel ditolak.
- `go test ./internal/service -run '^TestPostgresAuditTrail$' -count=1 -timeout=90s -v` — lulus; kelompok aksi sensitif, korelasi/metadata/privasi, awal inklusif/akhir eksklusif, filter/source/page dan akses owner. Enam rollback staf/harga/status/payment/refund/shift close mempertahankan snapshot tabel bisnis/audit tanpa perubahan parsial atau event sukses palsu.
- `go test ./internal/http -run '^TestPostgres(AuditHTTP|CookieAuthenticationLifecycle)$' -count=1 -timeout=90s -v` — kedua test lulus. HTTPS/cookie/sesi PostgreSQL nyata: anonymous 401, admin/cashier 403, owner 200, request ID caller tidak dipercaya, korelasi respons sesuai audit, metadata harga benar, union security, HEAD tanpa body, metode tulis 405 dan audit-failure session rollback.
- `go test ./... -count=1 -timeout=180s` — semua package selain service lulus; service menemukan satu assertion lama yang mengharapkan akses audit admin. Assertion disesuaikan kebijakan owner-only; `go test ./internal/service -run '^TestPostgresStaffAdministration$' -count=1 -timeout=90s -v` diulang dan kedua subtest lulus. Test lain dalam suite tersebut tidak melaporkan kegagalan; full suite tidak diulang tanpa kebutuhan.
- `go build ./...`, `go vet ./internal/domain ./internal/service ./internal/repository/... ./internal/http/... ./internal/migrate ./cmd/server` dan `node --check frontend/audit.js` — lulus. `node --test --test-name-pattern='cashier loads session' scripts/frontend-contract.test.cjs` — satu test regresi navigasi/sesi/DOM/logout lulus; bukan full frontend suite.
- Browser Edge headless lokal dengan mock API — desktop 1280 px, mobile emulation 390 px, tanggal WITA/aksi, pagination, kosong/error/reset, fokus, tabel geser tanpa overflow halaman, teks markup tidak menjadi HTML dan cashier guard lulus tanpa exception JavaScript. Screenshot desktop/mobile tersimpan di cache luar repository; mobile ditinjau. Bukan bukti browser yang terhubung database/perangkat Android fisik.
- gofmt, `git diff --check`, UTF-8 dan tautan dokumentasi lokal — bersih; diff/status ditinjau. Cluster PostgreSQL test 16.14 loopback 55448 — seluruh database test non-template dibersihkan (0 tersisa), cluster dihentikan. Tidak membaca `.env`, mengubah database operasional, commit, push atau deployment.
- Chime — chime-1.mp3 selesai diputar dua kali melalui MediaPlayer .NET, masing-masing mencapai posisi akhir 2,376 detik.

## Perubahan Penting

- `migrations/000012_audit_trail.up.sql` — request ID/JSONB constrained, guard baru dan indeks; fixture/seed mengikuti schema 12 tanpa mengubah migration lama.
- `internal/domain/audit.go`, service runtime/auth/staff/catalog/orders dan adapter audit — metadata minimum, korelasi, aksi login jelas dan pembatasan read_audit.
- `internal/repository/audit.go`, `internal/service/audit.go`, handler/DTO/router dan composition root — pembacaan gabungan dua sumber dan filter privat owner; test unit/integration/migration/HTTPS terarah.
- `frontend/audit.html/css/js` dan navigasi antrean — halaman owner dengan form/tabel responsif dan safe DOM, tanpa framework/dependency baru.
- [security.md](../security.md), [audit.md](../audit.md), API/database/architecture/README/backlog — kontrak, retensi, privilege, upgrade/rollback dan perintah verifikasi.

## Risiko atau Sisa Pekerjaan

- Schema operasional belum dimigrasikan. Upgrade nyata memerlukan backup/restore salinan dan writer schema 12; binary ≤11 tidak kompatibel untuk penulisan audit baru. Role runtime produksi tanpa superuser/DDL belum dibuktikan; trigger bukan perlindungan terhadap pemilik DDL.
- Retensi masih kebijakan minimum; arsip terverifikasi/purge maintenance belum diotomatisasi. Tidak melakukan penghapusan histori atau menonaktifkan guard. Pagination antar-request tidak membekukan snapshot saat event baru masuk.
- Android fisik, race instrumentation dan gate deployment/backup/acceptance produksi sebelumnya tetap terbuka. Tidak ada blocker implementasi Hari 17 yang diketahui; aplikasi belum dinyatakan layak produksi. Tahap berikutnya Hari 18 — fondasi UI dan tokens tema.
