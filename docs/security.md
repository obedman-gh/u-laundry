# Keamanan ULaundry — review terarah Hari 24

Diperbarui 27 November 2026, Asia/Makassar. Berlaku untuk server domain Go/PostgreSQL saat ini; belum merupakan penerimaan keamanan produksi menyeluruh. Tidak ada akun, password, sesi operasional, .env atau deployment pengguna yang diubah dalam pekerjaan ini. Audit lengkap, retensi dan upgrade schema 12 dijelaskan pada [audit.md](audit.md).

## Review terarah Hari 24

Perkembangan Hari 25: request/lifecycle kini memakai log JSON berlevel dengan request ID server, route template, method/status/duration dan klasifikasi aman; tidak mencatat URL/query/header/body, token atau identitas. `/api/health/live` publik tanpa DB, `/api/health/ready` dan alias `/api/health` memakai ping ber-timeout/draining. `/api/ops/metrics` privat hanya owner aktif dengan permission manage_configuration existing. Snapshot metrics bukan ledger/audit. Kebijakan panic/error/transport redaction, operasi dan batas bukti terbaru ada di [runbook.md](runbook.md) dan [laporan Hari 25](progress/day-25.md).

Scope: code review dan test lokal/fixture sintetis untuk auth, authorization, CORS, cookie/CSRF, header, input/limit, XSS, SQL injection, IDOR, tracking, struk, limiter, log, secret, dependency dan error leakage. Tidak ada penetration test server publik, performance test, perubahan DB atau deployment. Bagian Hari 09–17 di bawah adalah konteks kontrol existing; bukti aktual Hari 24 ada pada [laporan](progress/day-24.md).

### Temuan dan hasil

| Prioritas / area | Temuan sebelum perbaikan — hasil |
|---|---|
| High bersyarat: file statis / secret | `http.FileServer(http.Dir(STATIC_DIR))` dapat menyajikan `.env`, source atau backup bila root salah diarahkan atau file privat tersalin ke frontend. Kini hanya nama aset publik eksplisit pada `internal/http/assets.go` yang disajikan; direktori, file tak terdaftar dan metode tulis ditolak. Salah root tidak membuka arbitrary file. |
| Medium: header / XSS / clickjacking | Header keamanan belum menyeluruh pada HTML/aset/redirect. Kini CSP enforced, framing ditolak, nosniff, no-referrer, Permissions-Policy, COOP/CORP same-origin, noindex dan no-store berlaku pada router, termasuk error dan struk. |
| Medium: request limits | Decoder JSON sudah membatasi 64 KiB, tetapi logout, GET dan route lain belum memiliki batas body yang sama. Kini body semua API dibaca dengan batas 64 KiB sebelum auth/use case, termasuk Content-Length tak diketahui/chunked; terlalu besar mendapat 413, read failure 400 tanpa detail internal. Server menetapkan MaxHeaderBytes 16 KiB dan tetap memakai timeout existing. |
| Medium: public availability | Tracking belum dibatasi. Kini GET/HEAD berbagi limit 60/menit per peer TCP dan 300/menit per proses, dengan Retry-After; token tidak menjadi key limiter dan header forwarding tidak dipercaya. |
| Hardening: CORS | Default origin utama dan penolakan wildcard sudah tersedia pada config. Runtime juga menolak Origin kosong eksplisit/duplikat serta preflight method/header duplikat. Tidak ada fallback wildcard atau Allow-Credentials. Missing Origin tetap sah untuk read klien nonbrowser; mutasi tetap wajib Origin dan CSRF. |
| Hardening: JWT / startup | HS256 exact, signature, claims dan sesi aktif sudah diperiksa. Parser kini juga menolak base64 dengan pad bits nonkanonis. JWT_ALGORITHM opsional hanya boleh HS256; secret control/format character ditolak; URL HTTPS wajib cookie Secure di semua mode. |
| Frontend | Tidak ditemukan sink innerHTML/outerHTML/insertAdjacentHTML/document.write/eval pada frontend yang ditinjau. Nama/catatan/pesan memakai textContent, input.value atau DOM node; struk memakai html/template. Toggle password kini memakai classList agar konsisten dengan CSP tanpa style inline. Worker berganti ke versi 2026-11-27-2. |

Tidak ditemukan temuan Critical dalam scope ini. Temuan High bersyarat di atas ditutup pada kode; ini tidak menyatakan seluruh instalasi produksi bebas risiko. Allowlist bukan pertahanan terhadap penyerang yang sudah dapat mengganti berkas aplikasi yang diizinkan: direktori rilis harus hanya dapat ditulis operator tepercaya, tanpa symlink ke secret.

### Threat model ringkas

- Aset: password/hash staf, JWT/cookie/CSRF, data pribadi pelanggan, tautan tracking, struk, ledger dan audit. Trust boundary: browser atau pemegang token publik → reverse proxy/Tunnel → HTTP Go → service authorization → PostgreSQL. Konfigurasi/release filesystem berada di boundary operator.
- Penyerang anonim mencoba brute force, malformed/oversized input, token palsu dan resource ID tebakan. Browser berorigin lain mencoba CSRF/framing/exfiltration. Staf berizin rendah mencoba eskalasi role atau resource shift staf lain. Input pelanggan/layanan/catatan dapat membawa payload XSS atau SQL; server tidak menganggapnya kode.
- Auth cookie hanya menerima satu cookie host-only dan menolak Bearer. HS256 exact + issuer/audience/iat/exp/sub/role/jti, TTL terbatas, database session check dan pencabutan credential tetap wajib. Login generik dan limiter sebelum bcrypt tetap dipertahankan.
- Authorization dimuat ulang dari staf aktif pada service, bukan role UI/claim saja. Owner mengelola staf/audit; owner/admin untuk pengaturan harga/refund/laporan; staf aktif dapat membaca order/pelanggan/struk toko sesuai matrix. Membaca order staf lain di toko yang sama memang izin bisnis, bukan IDOR. Kasir tidak boleh membaca/mengubah shift kasir lain; owner/admin hanya dapat membaca shift lain.
- Origin exact dan HMAC CSRF wajib pada mutasi privat; login wajib origin utama. Cookie HttpOnly/Secure/SameSite dan CSP saling melengkapi. CSP membatasi sumber skrip/style/koneksi/worker/manifest ke self, menolak object/base/framing dan skrip inline/eval. HSTS max-age 31536000 hanya pada konfigurasi Secure + origin HTTPS, termasuk TLS termination; tidak memakai X-Forwarded-Proto atau includeSubDomains/preload.
- Tracking adalah bearer capability crypto/rand 256 bit, hash tersimpan, expiry 90 hari dan rotasi privat; fragment dibersihkan dari address bar dan tanpa persistence browser. Projection publik hanya kode/status/estimasi/pesan. Invalid/missing/expired token berkontrak 404 yang sama; ID order tidak menggantikan token. Struk tidak tersedia pada route publik dan respons privat no-store.
- SQL menggunakan parameter untuk nilai eksternal; gabungan string yang ditinjau adalah kolom/filter/potongan SQL tetap. DTO dan domain memvalidasi input, nilai status, ID, pagination dan uang integer; tidak mengubah schema atau transaksi keuangan dalam review ini. Error driver/domain mentah tidak diserialisasi; panic response juga generik dengan request ID buatan server.
- Log runtime yang ditinjau berisi outcome startup/shutdown dan error storage teredaksi; audit memakai metadata terkontrol, tanpa token/password/payload anonim. Token tracking tetap ada dalam path API, sehingga log reverse proxy/Cloudflare/analytics wajib disunting atau dimatikan untuk route itu. Belum ada pemeriksaan konfigurasi log infrastruktur nyata.

### Checklist keamanan

- [x] Auth positif/negatif: salah signature/algoritma, unsigned/expired/missing claims, token malformed/alias base64, duplicate cookie dan Bearer ditolak; cookie/CSRF/logout/replay diuji melalui unit/fake storage.
- [x] Authorization/IDOR: route privat GET/HEAD memerlukan sesi; role terkini dicek use case, owner-only staf/audit dan privacy projection tracking diuji. Struk meng-escape payload HTML. Matrix bisnis existing dipertahankan.
- [x] CORS: origin exact/allowlist, suffix attack/null/empty/duplikat dan preflight tak sah ditolak, tanpa wildcard/credential fallback; config production menolak HTTP/wildcard origin.
- [x] Input/limit/error: JSON type/content type/compression/duplicate key/unknown field/query/path divalidasi; body known/chunked dibatasi; error body dan panic tidak membocorkan detail internal.
- [x] Header dan frontend: CSP pada respons sukses/gagal/statis, HSTS mengikuti config; Edge loopback memblokir inline script, base injection dan fetch eksternal. Toggle password, manifest, worker, update, offline privacy dan print/mobile fixture tetap bekerja.
- [x] Rate limit: login peer/account/global existing dan tracking peer/global; X-Forwarded-For tidak menjadi jalan bypass. Tidak ada IP/token tracking dicatat oleh limiter publik; peer hanya key memori proses.
- [x] Secret/config: tidak mengubah `.env` operasional; Git melacak `.env.example`, bukan `.env`. Startup guard synthetic diuji tanpa mencetak nilai secret. Ini bukan hasil scanner secret seluruh histori Git.
- [x] Dependency integrity: `go mod verify` lulus; `go list -m all` memeriksa versi terpilih. Go 1.27.3, jwt/v5 5.3.1, godotenv 1.5.1, pq 1.12.3, x/crypto 0.58.0, x/sys 0.49.0, x/term 0.47.0; graph juga memilih x/net 0.60.0 dan x/text 0.43.0. Tidak ada perubahan dependency.
- [x] Static analysis tersedia: `go vet` pada config/security/http/app lulus. Tidak menginstal tool/dependency tambahan hanya untuk review ini.
- [ ] `govulncheck`/`staticcheck` tidak terpasang pada sesi ini. Integritas modul bukan vulnerability scan, dan versi di atas tidak dinyatakan latest atau bebas CVE. Jalankan `govulncheck ./...` dengan database advisori terkini pada lingkungan rilis yang menyediakannya.
- [ ] PostgreSQL auth/revocation integration tidak diulang Hari 24; tidak ada perubahan SQL/schema/session store. Acceptance browser–DB, role DB least privilege, TLS dan konfigurasi proxy/Tunnel/cache/log produksi tetap perlu diverifikasi sebelum exposure publik.
- [ ] Limiter masih per proses/reset saat restart dan membagi bucket peer bila proxy menyatukan alamat staf/pelanggan. Banyak instance memerlukan limiter bersama/kontrol edge; jangan langsung mempercayai X-Forwarded-For. Belum ada limit umum semua read API selain timeout, ukuran request, pagination dan izin.

Sebelum deployment, gunakan origin utama HTTPS, cookie Secure dan secret terpisah, layani hanya direktori rilis publik, bypass cache respons privat pada proxy/Cloudflare, serta pertahankan CSP tanpa unsafe-inline/unsafe-eval. Penambahan aset membutuhkan pembaruan allowlist; setiap rilis frontend/rollback membutuhkan versi worker baru. Tidak ada migrasi; rollback kode lama membuka kembali kelemahan yang ditutup, sehingga utamakan forward fix.

Referensi kontrol: [OWASP CSP](https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html) dan [Go vulnerability database](https://go.dev/doc/security/vuln/database). Bukti perintah dan batas aktual ada di [laporan Hari 24](progress/day-24.md); command frontend/browser tetap memakai script README existing, bukan tool verifikasi paralel baru.

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

Audit keamanan login_success dibuat atomik bersama registrasi sesi; logout atomik dengan pencabutan. Mulai Hari 17, audit bisnis credentials_verified menandai verifikasi credential/penandatanganan token sebelum registrasi sesi; staff_logged_in historis tetap sebagai verifikasi lama. Bila fase registrasi gagal, login HTTP gagal tanpa cookie dan tidak ada login_success/sesi palsu. Kegagalan credential dicatat di auth_events dalam transaksi terpisah agar tidak hilang bersama rollback autentikasi. Bila audit gagal, error dipropagasikan dan tidak diklaim berhasil.

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

## Audit trail hari 17

`GET /api/audit/events` dan `/audit.html` menyatukan projection bisnis/keamanan dengan filter WITA, aksi, source, actor/target/request ID serta pagination. Izin read_audit sekarang **owner aktif saja**, termasuk endpoint objek `/api/audit`; admin/cashier mendapat 403 meskipun claim lama menyebut owner. Ini mempersempit kontrak Hari 10; izin laporan dan katalog owner/admin tetap.

Schema 12 menyimpan korelasi server yang sama dengan X-Request-ID respons; caller tidak memilih ID. Command non-HTTP mendapat ID acak. Request ID historis tetap NULL. Metadata hanya version/status/role/active/unit_price bertipe dan diperiksa domain/constraint JSONB; tanpa payload, credential, token, secret atau snapshot PII. Login anonim tidak menyimpan username percobaan; permission denial tetap enum.

UPDATE/DELETE serta TRUNCATE kedua tabel audit ditolak trigger. Tidak ada HTTP mutasi/purge; role runtime tetap harus tanpa DDL/superuser. Transaksi gagal tidak menyisakan audit sukses bisnis; login_success/logout tetap atomik dengan sesi/revocation. Minimum retensi bisnis 24 bulan, keamanan 90 hari dan arsip privat terverifikasi dijelaskan di [audit.md](audit.md#retensi-operasional); belum ada penghapusan otomatis atau perubahan histori nyata. Penerimaan keamanan produksi, TLS dan role deployment tetap terbuka.
