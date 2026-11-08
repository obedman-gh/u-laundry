# Laporan Hari 10 — Authorization dan Pengelolaan Staf

## Tindakan — Hasil

- Pemeriksaan hasil hari 09 — repository awal bersih pada commit 85a51e5; cookie/CSRF/session tetap dipakai. Snapshot source sebelum perubahan tersimpan di backups/before-day10-20261109-065704 pada root workspace, tanpa .env.
- Permission matrix — owner/admin/cashier memiliki izin eksplisit untuk staf, layanan/harga, operasi order, diskon, payment, laporan, audit dan policy konfigurasi; pengelolaan staf/konfigurasi hanya owner. Role terkini dimuat dalam use case, bukan dipercaya dari UI/JWT/body.
- Use case/API/UI staf — owner dapat membuat akun aktif, menonaktifkan/mengaktifkan, mengganti role dan mereset password melalui staff.html/API. Password lama/hash tidak pernah dikembalikan; input password baru masked/dikonfirmasi/dibersihkan. Update memakai version, lock administrasi dan perlindungan self-lockout/owner aktif.
- Bootstrap pertama — command admin bootstrap-owner hanya untuk tabel staff kosong, dengan terminal tanpa echo atau pipe privat, policy password dan audit atomik; tidak ada credential hardcoded, endpoint bootstrap publik, recovery/reset existing atau seed bisnis. x/term ditambahkan khusus input terminal lintas platform.
- Audit — aksi akun atomik dengan perubahan dan pencabutan sesi; denial untuk permission penting dicatat terpisah sesudah transaksi/view selesai agar bertahan saat rollback, termasuk pada pool satu koneksi. Kegagalan recorder dipropagasikan; credential/token/payload tidak masuk audit.

## Verifikasi

- go test ./internal/service -skip '^TestPostgres' -count=1 -timeout=60s — lulus: matrix seluruh role/permission, create/change/reset/active, admin/cashier negative paths, version/self guard, hashing/validation, rollback dan bootstrap-closed.
- go test ./internal/http/... ./cmd/admin ./internal/migrate -skip '^TestPostgres' -count=1 -timeout=60s — lulus: actor/input boundary, unknown fields/role/active, response tanpa password, permission negative paths dan command stdin tanpa secret output. Tambahan HTTP real-use-case membuktikan role sumber terkini menolak request walau JWT fixture menyebut owner.
- go test ./internal/service -run '^TestPostgresStaffAdministration$' -count=1 -timeout=90s -v — dua skenario terarah PostgreSQL 16 lulus: bootstrap race, upgrade 5→6/idempotensi/akun-hash terjaga, constraint username/role, role-based denial audit, stale/self guard, role/reset/active revocation dan audit rollback tanpa mencabut sesi yang harus tetap aktif. Hanya DB disposable dan akun sintetis; 0 database test tersisa dan cluster test dihentikan.
- node --test scripts/frontend-contract.test.cjs — 8/8 lulus: alur login/kasir hari 09 serta owner-only UI, create tanpa mempertahankan password, safe DOM, CSRF/version dan stale selection.
- go build ./..., vet area service/postgres/HTTP/security/admin/server dan cross-build cmd/admin untuk Linux amd64 — lulus; gofmt/diff/status, tautan dokumen dan immutability migration 000001–000005 diperiksa. Tidak menjalankan full suite atau integration finansial umum.

## Perubahan Penting

- internal/service/staff.go/access.go/runtime.go dan repository ports/postgres/staff.go — workflow akun, matrix dan audit denial; handler tetap tanpa SQL.
- migrations/000006_staff_management.up.sql — version dan audit additions, tanpa mengubah akun/hash existing. Trigger revocation hari 09 dipertahankan.
- cmd/admin, go.mod/go.sum — bootstrap aman memakai x/term v0.47.0; x/sys transitive. Tidak ada password default/argumen plaintext.
- HTTP DTO/handler/router/server wiring, frontend/staff.html/staff.js dan navigasi antrean — pengelolaan owner dengan CSRF, pagination/refresh dan version conflict.
- README bagian akun awal, docs/api.md permission matrix/endpoint, security/database/backlog dan laporan ini — command, keputusan dan batas verifikasi.

## Risiko atau Sisa Pekerjaan

- Tidak menjalankan bootstrap, reset password, migration atau deployment pada akun/data operasional. Import/cutover data legacy, configuration module dan acceptance hari 11–30 tetap terpisah; aplikasi belum layak produksi menyeluruh.
- Input TTY tanpa echo memakai dependency resmi dan ditinjau/build, tetapi prompt terminal nyata Windows/Ubuntu belum diuji; jalur stdin diuji sintetis. Browser/mobile visual dan deployment belum diuji; frontend memakai test DOM sintetis.
- Invarian owner aktif ditegakkan pada use case dengan lock; penulis SQL manual di luar aplikasi tetap memerlukan prosedur terkontrol. Policy konfigurasi belum memiliki endpoint; tidak menambah modul spekulatif. Retensi audit, multi-instance limiter dan audit UI lengkap tetap scope lanjutan.
