# Laporan Hari 24 — Review Keamanan Terarah

## Tindakan — Hasil

- Review seluruh area yang diminta — auth, authorization/IDOR, CORS, cookie/CSRF, header, validasi/limit, XSS, SQL injection, tracking/struk, limiter, log/secret/dependency dan error leakage ditinjau pada kode dan lingkungan lokal. Tidak ditemukan Critical dalam scope; High bersyarat berupa paparan file privat saat salah STATIC_DIR ditutup dengan allowlist aset dan penolakan directory listing/backup.
- Header keamanan — CSP enforced tanpa inline/eval, DENY framing, nosniff, no-referrer, Permissions-Policy, COOP/CORP, noindex dan no-store berlaku menyeluruh; HSTS hanya mengikuti konfigurasi Secure + origin HTTPS. Error range aset tetap no-store.
- Batas HTTP dan CORS — body seluruh API maksimal 64 KiB termasuk chunked/logout/GET/route tak dikenal; MaxHeaderBytes 16 KiB. Origin kosong/duplikat dan preflight ambigu ditolak tanpa wildcard atau credential fallback.
- Auth/startup/frontend — HS256 exact existing dipertahankan dengan strict base64 decoding tambahan; JWT_ALGORITHM selain HS256, secret control/format character dan HTTPS tanpa cookie Secure ditolak. DOM existing tetap textContent/node/html-template escaped; toggle password memakai classList. Versi worker menjadi 2026-11-27-2.
- Tracking publik — GET/HEAD berbagi limit 60/menit per TCP peer dan 300/menit per proses dengan Retry-After, tanpa mempercayai forwarding header atau mencatat token. Projection minimal dan struk privat dipertahankan.
- Dokumentasi — threat model, hasil review menurut prioritas, checklist dan sisa verifikasi diperbarui pada docs/security.md; README dan .env.example mengikuti perilaku baru. Tidak mengubah DB/schema/dependency/.env operasional, tidak melakukan commit/push atau deployment publik.

## Verifikasi

- `go test ./internal/config ./internal/security ./internal/http/... ./internal/app -skip '^TestPostgres' -count=1 -timeout=60s` — lulus tanpa DB. Paket terdampak perubahan tambahan diuji ulang dan lulus; meliputi JWT none/signature/claims/expiry/base64 alias, cookie ambigu/Bearer, CSRF, CORS, body known/chunked, error redaction, tracking/receipt privacy, aset privat dan header. Header oversized HTTP loopback mendapat 431 sebelum health mengakses DB.
- `go test ./internal/service -run 'TestPermissionMatrix|TestStaffNegativeRoles|TestCustomerManagementAuthorization|TestTrackingRotationAuthorization|TestTrackingReportAndReadAuthorization|TestAuthenticationGenericFailure|TestDashboardReportAuthorization' -count=1 -timeout=60s` — lulus pada fixture/fake repository, mencakup izin terkini, kegagalan auth generik dan privasi.
- `node --test scripts/frontend-contract.test.cjs` — 31 test lulus, termasuk rendering teks berbahaya sebagai teks, transport cookie/CSRF, larangan tujuan cross-origin/Bearer dan toggle password tanpa style inline.
- `node scripts/pwa-browser-check.cjs` — Edge headless pada fixture loopback lulus menggunakan CSP yang sama dengan middleware Go. Inline script, base injection dan fetch eksternal diblokir; manifest, cache publik terbatas, offline tanpa data privat, update/rollback dua tab, print dan layout ponsel tetap lulus. Bukan browser–database acceptance.
- `go vet ./internal/config ./internal/security ./internal/http/... ./internal/app`, build cmd/server ke cache workspace, `go mod verify` dan `go list -m all` — lulus. Versi terpilih dicatat; tidak menyatakan dependency latest/bebas CVE.
- JavaScript syntax, format Go, Git diff/check/status dan dokumen diperiksa — tidak ada whitespace error; tidak menjalankan full suite, performance test atau pengujian server publik.

## Perubahan Penting

- `internal/http/assets.go`, router dan middleware — public asset boundary, MIME eksplisit agar nosniff/manifest konsisten Windows–Ubuntu, secure headers, batas body, CORS ambigu dan limiter tracking.
- `internal/security`, `internal/config`, `internal/app` — strict JWT decoding, startup guard dan batas header, beserta targeted negative tests.
- `frontend/login.js`, `frontend/sw.js`, script frontend/browser — DOM class handling, versi rilis baru dan bukti CSP/PWA lokal.
- `docs/security.md`, `README.md`, `.env.example` — threat model/checklist, konfigurasi, kontrak aset dan command verifikasi tetap satu jalur.

## Risiko atau Sisa Pekerjaan

- `govulncheck` dan `staticcheck` tidak terpasang; vulnerability scan advisori terkini belum dilakukan. `go mod verify` hanya membuktikan integritas modul. Tidak menginstal tool tambahan.
- PostgreSQL integration tidak diulang karena tidak ada perubahan repository/schema/session store. Browser–DB, role DB minimum, HTTPS/proxy/Tunnel/cache/log produksi dan Android fisik masih memerlukan acceptance sebelum exposure publik.
- Limiter per proses reset saat restart; proxy yang menyatukan alamat membagi bucket peer. Banyak instance membutuhkan kontrol bersama/edge. Tidak ada limiter umum semua read API. Jangan mencatat path token tracking pada proxy/analytics.
- Tambahan aset wajib masuk allowlist dan direktori rilis hanya boleh ditulis operator tepercaya, tanpa symlink ke secret. Setiap rilis frontend/rollback wajib mengganti versi worker. Tidak ada blocker implementasi yang diketahui dalam scope Hari 24; kesiapan produksi menyeluruh tidak diklaim.
