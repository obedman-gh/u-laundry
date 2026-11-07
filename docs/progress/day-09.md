# Laporan Hari 09 — Penguatan Login dan Session Management

## Tindakan — Hasil

- Audit autentikasi — JWT localStorage 24 jam tanpa issuer/audience/iat/role/session revocation diganti cookie HttpOnly dengan registrasi sesi PostgreSQL; akun/password operasional tidak diubah. Snapshot source sebelum perubahan tersedia di backups/before-day09-20261108-033237 pada root workspace, tanpa .env.
- Policy token/credential — HS256 eksak, issuer/audience/iat/exp/identity-role-jti wajib, default lifetime 8h (15m–12h), secret tervalidasi sebelum startup, generic failure dan dummy bcrypt untuk username tidak dikenal. Policy akun baru 12 karakter/72 byte tersedia melalui hash entry point, tanpa memaksa password login existing berubah.
- Session dan request — cookie host-only HttpOnly/Secure production/SameSite, CSRF HMAC per sesi dan origin exact, endpoint session/logout, revocation persisten serta trigger password/role/status/username. Fingerprint credential menutup race saat login versus perubahan password.
- Rate limit/audit — limiter bounded dan concurrency-safe per peer/account/global, 429 Retry-After; event sukses/gagal/terbatas/logout tanpa credential/token/username input. Session creation/logout dan audit keamanan atomik; audit failure tidak mengirim cookie sukses.
- Kompatibilitas kasir — login/daftar memakai cookie same-origin, membersihkan token lama, mengambil sesi/CSRF, logout dan render text DOM aman; palet tetap. Token lama perlu satu login ulang; tidak ada fallback Bearer.

## Verifikasi

- go test ./internal/security ./internal/http/... ./internal/config ./internal/platform ./internal/app -count=1 -timeout=60s — lulus; termasuk valid/expired/wrong algorithm/issuer/audience, iat/exp/identity-role-jti hilang atau salah, secret startup hilang, cookie flags, CSRF, unauthorized, brute-force/concurrency/capacity/recovery, audit redaction, logout/replay dan error rollback.
- go test ./internal/service -run '^TestAuthentication' -count=1 -timeout=45s dan go test ./internal/migrate -run '^TestCatalog$' -count=1 -timeout=45s — lulus; login generik/audit dan katalog lima migration.
- go test ./internal/http -run '^TestPostgresCookieAuthenticationLifecycle$' -count=1 -timeout=90s -v dengan ULAUNDRY_TEST_ADMIN_DSN pada cluster PostgreSQL 16 test baru — lulus: upgrade 4→5/idempotensi/history/legacy row, HTTPS cookie/session/CSRF/logout/replay/restart, role/active/password revocation permanen dan snapshot credential lama, audit failure rollback/no cookie, append-only dan event tanpa credential. Database disposable dibersihkan (0 tersisa); cluster test dihentikan.
- node --test scripts/frontend-contract.test.cjs — 5/5 lulus: pesan/fallback login, pembersihan localStorage tanpa JWT baru, kegagalan jaringan, pemuatan kasir/cookie/safe text dan logout-CSRF.
- go build ./... dan vet terarah security/HTTP/config/platform/service/postgres/app/server — lulus; gofmt/diff/status dan dokumen diperiksa. Tidak menjalankan full suite, browser nyata atau deployment.

## Perubahan Penting

- internal/security/, platform/workflow.go, domain/password.go dan config — codec JWT, cookie/CSRF, limiter, password policy dan konfigurasi lifetime/issuer/audience/limit.
- internal/repository/postgres/sessions.go dan migrations/000005_staff_sessions.up.sql — sesi/audit persisten, pemeriksaan fingerprint serta pencabutan otomatis; SQL 000001–000004 tidak diubah.
- service/auth.go, handler/middleware/router/app dan cmd/server — wiring sesi server, generic authentication dan audit tanpa menyisipkan SQL ke handler.
- frontend/login.html/index.html, targeted tests, .env.example, README/API/architecture/database/backlog dan docs/security.md — migrasi klien, command, keputusan dan batas operasional.

## Risiko atau Sisa Pekerjaan

- Bootstrap/import akun domain dan user management tetap hari 10; tidak membuat akun/password tetap, mengubah .env, melakukan migration data nyata atau deployment. Aplikasi belum siap produksi menyeluruh.
- Limiter per proses, reset saat restart; deployment multi-instance memerlukan limiter bersama. Peer reverse proxy berbagi kuota sampai trust-proxy policy tersedia. Retensi audit/cleanup sesi belum otomatis.
- CSRF/HttpOnly tidak menggantikan review XSS/CSP dan keamanan keseluruhan; frontend kasir lengkap, browser Android/HTTPS produksi, DB TLS produksi, race detector dan release checks belum diverifikasi. Password blocklist terbatas; tidak mengklaim pemeriksaan database password bocor.
