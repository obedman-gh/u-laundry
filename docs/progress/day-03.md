# Laporan Hari 03 — Konfigurasi Bertipe dan Database Lifecycle

## Tindakan — Hasil

- Konfigurasi startup bertipe dan tervalidasi sekali — mode, address/timeouts, static path, PostgreSQL DSN/komponen, SSL/CA, pool/lifetime/deadline, JWT/cookie secret/flags, trusted origins, log level dan public URL tersedia.
- Guard production — menolak secret kosong/pendek/placeholder, JWT-cookie key sama, origin/public URL non-HTTPS, wildcard, cookie tidak Secure, SSL selain verify-full/CA kosong, password DB lemah dan konfigurasi pool tidak konsisten sebelum membuka DB/listener.
- Database initialization — dependency/error dikembalikan, MaxOpenConns/MaxIdleConns/ConnMaxLifetime diterapkan, PingContext dan connect timeout aktif, failure/cancel menutup pool; tidak ada DB global atau log.Fatal package.
- Config driver eksplisit — credential tidak dirangkai ke DSN string, override PG*/passfile/service tidak dibaca, error teredaksi tetap mendukung errors.Is/As.
- Integrasi origin/config — CORS allowlist tanpa fallback wildcard, preflight valid 204 sebelum router/auth; startup/shutdown normal memakai JSON log sesuai level. Cookie config disiapkan tanpa mengklaim cookie session sudah aktif.
- Migrasi utilitas lama — adapter `config/database.go` dihapus; tiga executable menerima dependency lokal, menutup DB, ditolak pada production dan tidak lagi mencetak password bootstrap.

## Verifikasi

- `go test ./internal/config ./internal/platform ./internal/http ./internal/http/middleware ./internal/app -count=1 -timeout=45s` — lima package lulus dengan map/connector sintetis dan listener loopback, tanpa `.env` developer/DB operasional.
- Config tests — development/test/production valid; invalid mode/address/duration/pool/secrets/SSL/origin/cookie/log, konflik DSN/komponen, escaping credential, alias origin dan error redaction ditolak sesuai kebijakan.
- DB/lifecycle/HTTP tests — pool max-open/max-idle, ping deadline/cancel/cleanup/cause/redaction, driver config eksplisit, graceful/forced shutdown/close failure, sembilan route/auth boundary, aset HTML dan CORS/preflight terverifikasi terarah.
- `gofmt -w cmd internal`, `go build ./...` — berhasil dengan Go 1.27.3, dependency lokal dan cache di workspace.
- Review konfigurasi contoh, dokumentasi, Git diff/status — tidak mengubah `.env`, dependency, schema, handler bisnis atau tema.
- Chime `sounds/chime-1.mp3` — pemutar Windows menyelesaikan playback dua kali.
- Tidak menjalankan full suite/PostgreSQL integration: lifecycle inisialisasi/pool/cleanup dibuktikan dengan driver fake, tanpa perubahan SQL bisnis. TLS/authentication PostgreSQL nyata, CA infrastruktur dan restore tidak diklaim terverifikasi.

## Perubahan Penting

- `internal/config/` — snapshot config, parsing URL DSN, kebijakan production dan unit tests murni tanpa cwd/environment nyata.
- `internal/platform/` — connector PostgreSQL bertipe, pool, timeout/cleanup dan unit tests; `cmd/server/`, `internal/app/` — fail-fast bootstrap dan penerapan timeout/read-header/origin/log level.
- `internal/http/` — trusted-origin middleware dan preflight global beserta positive/negative tests.
- `cmd/legacy/`, penghapusan `config/database.go` — dependency lokal menggantikan adapter global; utilitas tetap menunggu pengganti migration/bootstrap aman.
- `.env.example`, `README.md`, `docs/target-architecture.md`, audit/backlog — setting lengkap, aturan upgrade `.env`, batas verifikasi dan status tindak lanjut diperbarui.

## Risiko atau Sisa Pekerjaan

- `.env` lokal tidak dimigrasi otomatis; owner perlu mengisi JWT/cookie secret yang memenuhi kebijakan dan setting TLS production. Konfigurasi lama yang tidak aman sengaja gagal startup.
- Sertifikat/hostname/CA, autentikasi PostgreSQL dan koneksi infrastruktur aktual belum diuji. Belum ada migration berversi atau bootstrap owner aman; legacy command dilarang dipakai untuk data nyata.
- Query/Ping handler lama belum memakai request context; forced HTTP close tidak menjamin query berhenti. Cookie config belum menjadi session cookie/CSRF; exact JWT algorithm/claims/role masih pekerjaan hardening berikutnya.
- Risiko uang/idempotensi, create/schema mismatch, filter status, XSS/localStorage dan detail/struk publik baseline masih terbuka. Aplikasi belum dinyatakan siap produksi.
