# Keamanan autentikasi dan authorization ULaundry — hari 10

Diperbarui 9 November 2026, Asia/Makassar. Berlaku untuk server domain Go/PostgreSQL saat ini; belum merupakan penerimaan keamanan produksi menyeluruh. Tidak ada akun, password, sesi operasional, .env atau deployment pengguna yang diubah dalam pekerjaan ini.

## Audit dan keputusan penyimpanan

Sebelumnya: login generik dan startup secret validation tersedia, tetapi JWT 24 jam disimpan di localStorage; parse hanya HS256/expiration/subject. Belum ada issuer/audience/issued-at/role/session ID wajib, rate limit, logout/revocation server atau audit kegagalan anonim. Login sukses tercatat di audit bisnis dengan actor staf; audit tersebut tidak cocok untuk username yang tidak dikenal.

Keputusan: aplikasi satu origin memakai JWT hanya dalam cookie HttpOnly, dengan registrasi sesi persisten PostgreSQL. Cookie tidak dibaca JavaScript dan JWT tidak dikembalikan sebagai JSON. localStorage token lama dihapus saat membuka login/daftar. UI memakai GET /api/session untuk identitas, expiry dan token CSRF yang disimpan hanya dalam memori halaman. Tidak memakai refresh token, silent refresh, antrean offline atau fallback Bearer. Pilihan mengikuti [panduan sesi OWASP](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html); HttpOnly membatasi pembacaan cookie, sedangkan XSS tetap dapat menjalankan request sebagai pengguna dan memerlukan pertahanan terpisah.

Peralihan ini memerlukan satu login ulang setelah pemasangan. Token localStorage lama tidak diterima, termasuk jika dipindahkan ke cookie; token tanpa claims/registrasi sesi baru ditolak. Data/role/password staf tidak direset. Bootstrap owner domain dan manajemen staf kini tersedia pada hari 10; akun/data legacy tetap memerlukan import/cutover terpisah.

## Token, konfigurasi dan sesi

- Algorithm tetap HS256 secara eksak. Parser memakai WithValidMethods, issuer, audience, WithIssuedAt dan WithExpirationRequired dari [golang-jwt v5.3.1](https://pkg.go.dev/github.com/golang-jwt/jwt/v5@v5.3.1). Pemeriksaan eksplisit juga mewajibkan iat, exp, sub, username, role sah dan jti acak; iat masa depan, exp tidak sesudah iat dan lifetime melampaui batas ditolak. Tidak ada leeway tersembunyi; sinkronkan clock server.
- JWT_ISSUER default origin PUBLIC_BASE_URL kanonis; JWT_AUDIENCE default ulaundry-staff; SESSION_TTL default 8h, dapat 15m–12h dalam detik penuh. Identitas memakai sub ID staf, username dan role owner/admin/cashier; nama/kontak tidak dimasukkan. Tidak ada kebijakan memperpanjang sesi otomatis.
- JWT_SECRET dan COOKIE_SECRET tetap wajib acak/non-placeholder minimal 32 byte pada startup, sebelum koneksi DB/listener. Pada production kedua secret berbeda; production juga tetap membutuhkan HTTPS, Secure cookie dan kebijakan DB TLS yang sudah ada. Jangan mencetak nilai konfigurasi.
- Cookie production bernama __Host-ulaundry_session dengan Secure, HttpOnly, Path=/, tanpa Domain, expiry/Max-Age sesuai sisa lifetime dan SameSite sesuai konfigurasi (default Lax). Nonproduction HTTP memakai ulaundry_session dan Secure=false hanya jika dikonfigurasi. SameSite=None tetap membutuhkan Secure dan perlindungan CSRF penuh; bukan dukungan frontend lintas origin.
- DB menyimpan SHA-256 jti, ID/username/role, fingerprint SHA-256 hash credential dan waktu; tidak menyimpan JWT mentah atau token CSRF. Setiap request memvalidasi signature/claims, registrasi/revocation dan staf aktif beserta role/username/fingerprint saat ini. Use case tetap memuat role/active untuk izin bisnis.
- Sesi tetap dapat dibaca instance manager baru melalui database. Cookie tidak dibuat bila transaksi registrasi/audit gagal. Fingerprint asal verifikasi credential harus cocok dengan staf yang dikunci FOR SHARE saat registrasi; perubahan password di antara verifikasi dan registrasi tidak menghasilkan sesi dari credential lama.

## CSRF dan logout

SameSite merupakan lapisan tambahan, bukan pengganti token CSRF; lihat [panduan CSRF OWASP](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html).

- Login JSON memerlukan satu Origin yang sama persis dengan PUBLIC_BASE_URL dan menolak Sec-Fetch-Site cross-site. Login tidak membutuhkan sesi sebelumnya. Klien nonbrowser harus mengirim Origin yang benar secara eksplisit.
- Mutasi privat, termasuk logout, memerlukan cookie valid, satu X-CSRF-Token serta Origin exact yang sama. Token berupa HMAC-SHA256 jti dengan COOKIE_SECRET, dibandingkan secara constant-time; tidak memakai cookie readable JavaScript. Token dikembalikan saat login/GET /api/session melalui respons no-store. GET/HEAD tidak membutuhkan CSRF; seluruh izin resource tetap diperiksa server.
- POST /api/logout mencabut sesi dalam transaksi yang sama dengan audit logout, lalu menghapus cookie dengan atribut/path yang sama. Cookie lama yang disalin tidak bisa dipakai ulang; request setelah logout mendapat 401. Kegagalan storage tidak dilaporkan sebagai logout sukses dan tidak menghapus cookie seolah revocation sudah berhasil.
- Trigger database mencabut sesi lama saat password_hash/username/role/active staf berubah. Mengembalikan nilai lama atau mengaktifkan ulang akun tidak menghidupkan kembali cookie yang dicabut. Tidak ada UI pengelolaan semua perangkat pada tahap ini.
- CORS allowlist tetap dipertahankan, tetapi cookie login/mutasi hanya untuk origin utama; tidak menambahkan Access-Control-Allow-Credentials lintas origin. CORS bukan autentikasi atau pengganti CSRF. Header Authorization ditolak pada sesi cookie, sehingga tidak ada fallback token dengan kebijakan lebih lemah.

## Login, password dan pembatas percobaan

Username tidak ada, password salah dan staf nonaktif menggunakan kegagalan autentikasi generik. Username tak dikenal menjalankan dummy bcrypt; staf nonaktif juga tetap diperiksa password sebelum penolakan. Ini mitigasi timing dasar, bukan klaim seluruh waktu jaringan/DB/variasi cost hash lama konstan. Error internal tidak membocorkan pesan driver, hash, password atau JWT. Request malformed tetap memakai error boundary 400/413/415 sebelum verifikasi credential.

Password akun baru/reset melewati domain.ValidateNewPassword dan platform.Passwords.Hash: minimal 12 karakter Unicode, maksimal 72 byte bcrypt, tanpa control/format character atau spasi tepi; pola berulang dan daftar kecil password umum ditolak. Frasa sandi diperbolehkan tanpa kewajiban komposisi karakter. Hash memakai bcrypt DefaultCost (10). Policy bukan daftar seluruh password bocor, dan tidak mengubah akun existing atau mewajibkan password login lama memenuhi panjang baru. API staf dan bootstrap hari 10 memakai entry point hash ini; tidak mengubah password existing otomatis.

Limiter concurrency-safe menghitung semua percobaan credential, termasuk sukses, sebelum bcrypt: default peer TCP 10/1m (LOGIN_RATE_LIMIT/LOGIN_RATE_WINDOW), identifier akun case-folded 10/15m, global proses 100/1m. Respons 429 membawa Retry-After generik. Username/IP hanya menjadi key HMAC di memori, tidak ditulis ke audit/log. Header X-Forwarded-For tidak dipercaya. Bucket maksimal 4096, kadaluwarsa dibersihkan dan bucket aktif tidak dieviction untuk membuka limit. Event login_limited dibatasi pada transisi pertama ke kondisi terblokir, bukan satu tulisan DB setiap request terblokir.

Batas: limiter per proses dan reset saat restart. Untuk banyak instance gunakan limiter bersama; deployment reverse proxy yang membuat semua staf terlihat sebagai satu peer akan berbagi batas peer. Belum ada trust-proxy policy untuk membaca header forwarding. Jangan menaikkan limit atau mempercayai header klien sebagai jalan pintas. Limit bukan lockout permanen akun.

## Audit dan storage

Migration 000005 additive menambah staff_sessions dan auth_events serta trigger pencabutan; SQL versi 000001–000004 tidak diubah. auth_events append-only berisi kind (login_success/login_failure/login_limited/logout), actor ID nullable, request ID server dan waktu. Kegagalan anonim tidak dibuatkan akun fiktif dan tidak mencatat username yang dikirim, peer/IP, password, token maupun hash credential.

Audit keamanan login_success dibuat atomik bersama registrasi sesi; logout atomik dengan pencabutan. Audit staff_logged_in existing tetap menandai verifikasi credential sukses pada transaksi use case, sebelum fase registrasi sesi. Bila fase berikut gagal, login HTTP tetap gagal dan tidak mengirim cookie; auth_events tidak mengklaim sesi berhasil. Kegagalan credential dicatat di auth_events dalam transaksi terpisah agar tidak hilang bersama rollback autentikasi. Bila audit gagal, error dipropagasikan dan tidak diklaim berhasil.

Runtime memerlukan hak SELECT/INSERT/UPDATE staff_sessions, INSERT auth_events beserta sequence identity, dan hak bisnis existing. Tidak memerlukan PostgreSQL superuser atau ekstensi baru. Trigger berjalan dengan hak invoker; account-management writer nanti juga memerlukan hak pencabutan sesi. auth_events tidak boleh UPDATE/DELETE melalui alur runtime. Retensi audit/cleanup sesi kedaluwarsa belum diotomatisasi; jangan truncate histori atau menghapus sesi aktif untuk setup biasa.

## Pemasangan dan verifikasi

Dari F:\project\ulaundry\dev, pertahankan .env existing; isi setting baru hanya jika default perlu disesuaikan. Pastikan PUBLIC_BASE_URL sama dengan origin yang dipakai kasir. Jalankan migration pada salinan/backup terverifikasi terlebih dahulu, kemudian prosedur upgrade terotorisasi:

```powershell
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/server
```

Tidak ada migration operasional atau server aplikasi yang dijalankan pada tugas hari 09. Rollback kode tidak memerlukan drop tabel auth baru; versi lama yang mengaktifkan kembali Bearer/localStorage adalah kemunduran keamanan dan membutuhkan peninjauan. Jangan mengedit checksum migration yang sudah diterapkan.

Command verifikasi terarah:

```powershell
go test ./internal/security ./internal/http/... ./internal/config ./internal/platform ./internal/app -count=1 -timeout=60s
go test ./internal/service -run '^TestAuthentication' -count=1 -timeout=45s
go test ./internal/migrate -run '^TestCatalog$' -count=1 -timeout=45s
node --test scripts/frontend-contract.test.cjs
go build ./...
```

Integration opt-in pada cluster test khusus (bukan DB operasional), menggunakan database postgres sebagai administrasi untuk membuat/menghapus database acak:

```powershell
$env:ULAUNDRY_TEST_ADMIN_DSN = 'postgres://TEST_ADMIN@127.0.0.1:55439/postgres?sslmode=disable'
go test ./internal/http -run '^TestPostgresCookieAuthenticationLifecycle$' -count=1 -timeout=90s -v
```

Bukti: PostgreSQL 16 test terisolasi + httptest HTTPS memverifikasi upgrade 4→5/idempotensi, row legacy tetap, generic failure dan audit, cookie flags/session, CSRF/logout/replay/restart, pencabutan role/active/password dan race snapshot credential lama, audit-failure rollback/no cookie, append-only audit dan ketiadaan credential pada event. Lima test JavaScript memeriksa cookie fetch, penghapusan token lama, pesan login, pemuatan kasir/teks aman dan logout-CSRF. Bukan bukti browser Android/secure context nyata, DB TLS produksi, multi-instance limiter, keseluruhan review XSS/CSP, atau kesiapan produksi. Tidak menjalankan full suite, race detector atau deployment.

## Authorization dan akun hari 10

Owner saja dapat mengelola staf; admin/cashier tidak dapat menaikkan role melalui API/claim/header. Matrix lengkap ada di [api.md](api.md#permission-matrix-hari-10). Perubahan staf memegang transaction advisory lock, memuat actor ulang, memeriksa version, menjaga owner aktif dan mencatat audit atomik dengan update/credential revocation. Bootstrap hanya command untuk tabel staff kosong, memakai input terminal tanpa echo atau pipe privat; tidak ada bootstrap HTTP, password bawaan atau recovery tersembunyi.

auth_events kini juga mempunyai kind access_denied dan kolom permission terkontrol. Dengan decorator WithAccessAudit pada composition root, denial direkam sesudah transaksi/view ditolak selesai; audit tetap ada meskipun perubahan bisnis rollback. Password/hash/token dan payload penolakan tidak disalin. Bila recorder gagal, kegagalan dipropagasikan. Mutation audit staff_created/staff_activated/staff_deactivated/staff_role_changed/staff_password_reset dan staff_bootstrapped tetap di audit_events append-only, tanpa password lama/baru.

Scope yang belum diuji tetap browser/terminal nyata, DB TLS produksi dan deployment. Tidak ada perubahan/reset credential operasional pada tugas hari 10; lihat [laporan hari 10](progress/day-10.md).
