# ULaundry POS & Management System

ULaundry adalah aplikasi operasional laundry di Perumahan Viola milik Obed Erwin Manambe. Stack: Go (`net/http`), PostgreSQL, HTML, CSS, dan JavaScript tanpa framework frontend besar.

Status per 11 November 2026: **katalog layanan, histori harga dan snapshot order tahap 12 tersedia, belum layak produksi**. Lihat [arsitektur](docs/architecture.md), [kontrak API dan perubahan kompatibilitas](docs/api.md), [aturan bisnis](docs/business-rules.md), [database](docs/database.md), [audit baseline](docs/audit-baseline.md), [kriteria penerimaan](docs/acceptance-criteria.md), [backlog](docs/backlog-30-days.md), [laporan hari 12](docs/progress/day-12.md) dan [keamanan autentikasi](docs/security.md).

## Implementasi saat ini

- Owner dapat mengelola staf melalui /staff.html; API/use case menegakkan matrix izin dan audit. Bootstrap akun pertama tanpa password hardcoded tersedia.
- Login staf generik dengan bcrypt/JWT HS256, claims wajib, cookie HttpOnly, sesi PostgreSQL, CSRF, logout/revocation dan rate limit; role dimuat ulang pada use case.
- Create/edit/search customer dan riwayat order, configure service, create order dengan perhitungan katalog server, status/version dan cancellation terkontrol, serta daftar/detail berfilter/pagination.
- Owner/admin mengelola layanan kg/item, harga, durasi, aktif dan urutan melalui `/services.html`; histori revisi dan snapshot order menjaga total transaksi lama.
- Endpoint payment terpisah, payment-ledger-audit satu transaksi, replay key identik tanpa posting ulang; `Sudah Lunas` bukan status operasional.
- Struk staf terautentikasi dan HTML escaping; printer fisik 58/80mm belum diverifikasi. Tracking publik memakai token acak dengan hash tersimpan dan projection minimal.
- Data laporan buku kas per rentang tanggal (default hari berjalan WITA), hanya owner/admin dan diaudit.
- Halaman login dan daftar antrean; belum tersedia UI POS lengkap, pembayaran, shift, dan laporan.
- Migration 000005 menambah sesi/audit autentikasi dan pencabutan saat credential/role/status berubah; tidak mengubah data legacy.
- `/api/health` memeriksa database (503 bila tidak tersedia); belum ada pemisahan liveness/readiness.
- Error API seragam `error.code/message/request_id`, request ID server, batas JSON 64 KiB/UTF-8, penolakan field asing/duplikat, 404 API dan 405 dengan Allow; pesan login memahami kontrak baru.

Struktur: `cmd/server/` menyusun pool, runtime clock/ID/token, repository dan use case; `internal/app/` lifecycle; `internal/config/` konfigurasi; `internal/platform/` adapter teknis; `internal/http/` transport; `internal/service/` workflow/authorization/transaksi; `internal/domain/` aturan; `internal/repository/postgres/` SQL. Handler tidak lagi menerima DB, mengatur transaksi atau menyusun SQL. `models/` DTO lama tidak lagi dipakai handler; `frontend/` tetap aset statis. Runner migration tidak otomatis dipanggil server.

Use case dipisah menjadi Customers/Catalog/Orders/Payments/Tracking/Reports/Auth/Health, dengan clock/ID/token generator yang dapat diganti untuk test. Payload create order menolak harga/total klien; snapshot katalog dan diskon sah dihitung domain. Kegagalan audit menyebabkan rollback perubahan pada use case, bukan keberhasilan parsial. Pilihan kebijakan domain hari 05 tetap perlu ditinjau sebelum aktivasi operasional.

Migration **000008** menambah urutan layanan dan histori revisi dengan baseline nilai existing. `up` menerapkan delapan versi; SQL existing tetap immutable. **Server sekarang memakai tabel domain, bukan pesanan/kasir/keuangan lama.** Data legacy tetap, tetapi tidak otomatis terlihat/diimpor atau dipakai login. Sebelum deployment data nyata wajib backup/restore salinan, mapping/import akun/status/uang, rekonsiliasi dan satu writer. Bootstrap owner domain aman tersedia lewat cmd/admin; utilitas setup-kasir lama tidak membuat akun domain. Jangan menganggap perubahan kode ini sudah melakukan cutover data operasional. Seed katalog domain terpisah tersedia di scripts/catalog-demo.sql; scripts/demo.sql tetap khusus legacy.

## Prasyarat dan setup lokal

- `go.mod` menetapkan Go **1.27.3**; toolchain lokal itu berhasil membangun seluruh package pada hari 02.
- PostgreSQL lokal/VM dan database kosong khusus pengembangan, bukan database operasional.
- Jalankan dari root repository (`F:\project\ulaundry\dev` pada workspace ini).

```powershell
git clone https://github.com/obedman-gh/u-laundry.git
cd u-laundry
Copy-Item .env.example .env
```

Untuk checkout yang sudah memiliki `.env`, pertahankan file itu dan tambahkan setting baru secara lokal; jangan menimpanya dengan contoh. `.env` tidak dibaca atau diubah selama pekerjaan hari 03. Sistem environment mengungguli `.env`; file tidak ada dibolehkan, tetapi file rusak/tidak terbaca menggagalkan startup.

Isi koneksi DB pengembangan sesuai database terisolasi, lalu isi `JWT_SECRET` dan `COOKIE_SECRET` dengan dua secret acak independen minimal **32 byte** masing-masing (misalnya 32 byte dari generator kriptografis yang dikodekan Base64). Tidak ada secret bawaan; nilai kosong, pendek, placeholder umum dan karakter berulang ditolak. Jangan gunakan secret contoh/test dalam deployment. Cookie HttpOnly/session PostgreSQL dan CSRF kini aktif; JWT tidak dikembalikan ke JavaScript atau disimpan di localStorage. Login lama memerlukan satu login ulang. SESSION_TTL default 8h; JWT_ISSUER default PUBLIC_BASE_URL, JWT_AUDIENCE ulaundry-staff. Lihat docs/security.md untuk policy dan limit.

Pilihan koneksi: komponen `DB_HOST/DB_PORT/DB_USER/DB_PASS/DB_NAME/DB_SSLMODE/DB_SSLROOTCERT` atau `DB_DSN` berupa URL `postgres://`/`postgresql://`. Keduanya tidak boleh dicampur. DSN dibatasi satu host TCP, user dan database, dengan query hanya `sslmode`/`sslrootcert` (tanpa duplikasi); password/karakter khusus harus percent-encoded. Keyword DSN, multi-host, Unix socket dan opsi query lain belum didukung. Pool dan timeout tetap menggunakan variabel terpisah. Driver menerima struktur eksplisit, sehingga `PG*`, pgpass, service file atau penyisipan string tidak mengganti setting tervalidasi.

Semua variabel beserta default/kebijakan ada di [.env.example](.env.example):

| Area | Setting dan kebijakan |
|---|---|
| Mode | `APP_ENV=development/test/production`; default development |
| HTTP | `SERVER_ADDRESS=127.0.0.1:8080`, `STATIC_DIR=./frontend`; read-header 5s, read/write 10s, idle 15s, shutdown 5s; durasi positif, read-header ≤ read |
| Database | Default development host 127.0.0.1, port 5432, user/database ulaundry, SSL disable; contoh memakai nama DB ulaundry_dev |
| Pool | `DB_MAX_OPEN_CONNS=10`, `DB_MAX_IDLE_CONNS=5`, `DB_CONN_MAX_LIFETIME=30m`; max-open ≥ 1, idle 0..max-open |
| DB startup | `DB_CONNECT_TIMEOUT=5s`, `DB_PING_TIMEOUT=5s`; positif, connect ≤ ping; pool ditutup saat ping gagal/cancel atau saat app selesai |
| Origin | `PUBLIC_BASE_URL=http://localhost:8080`; `TRUSTED_ORIGINS` dipisah koma, default hanya origin public URL; wajib mencakup public URL, tanpa wildcard/user/path/query/fragment |
| Kompatibilitas origin | `ALLOWED_ORIGIN` masih alias satu origin; ditolak jika digabung dengan TRUSTED_ORIGINS |
| Cookie | Secret ≥ 32 byte, `COOKIE_SAME_SITE=lax/strict/none`, `COOKIE_SECURE=false` untuk development; none memerlukan Secure |
| Logging | `LOG_LEVEL=debug/info/warn/error`; startup/shutdown JSON memakai level ini; request logging lengkap masih backlog |

**Production wajib** mengatur `APP_ENV=production`, public URL HTTPS eksplisit, seluruh trusted origin HTTPS, JWT/cookie secret berbeda, `COOKIE_SECURE=true`, koneksi host/user/database eksplisit dan password DB non-placeholder minimal **16 byte**. SSL wajib `verify-full` dan `DB_SSLROOTCERT` berupa CA PEM yang dapat dibaca atau `system` untuk trust store OS. Mode `disable`, `require`, dan `verify-ca` ditolak pada production; nilai `allow/prefer` ditolak pada semua mode. Validasi gagal sebelum koneksi DB/listener dibuka. File CA, sertifikat server/hostname dan konektivitas diperiksa saat startup ping; TLS terhadap infrastruktur nyata belum diuji hari 03. Jangan menurunkan SSL policy untuk membuat startup production lolos.

CORS sekarang memeriksa origin request persis terhadap allowlist dan memakai `Vary: Origin`; origin asing ditolak 403, tanpa wildcard fallback. Preflight dengan origin/method/header diizinkan mendapat 204 sebelum auth/router; method yang diizinkan GET/HEAD/POST/PUT, header Content-Type/Authorization/Idempotency-Key. Header respons X-Request-ID/Deprecation/Link/Location dapat dibaca trusted origin. Request tanpa Origin tetap mengikuti autentikasi route; CORS tidak melindungi detail/struk publik dari akses langsung.

Siapkan database pengembangan kosong dan isi konfigurasi DB yang sesuai (contoh `DB_NAME=ulaundry_dev`). Perintah skema resmi dari root repository:

```powershell
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/migrate status
```

Runner hanya memerlukan konfigurasi `APP_ENV`/DB, dengan kebijakan SSL/credential DB production yang sama seperti server; tidak memerlukan JWT/cookie secret. Environment mengungguli `.env`. Output mencantumkan versi, `pending`/`applied`, waktu UTC dan nama file. Seluruh versi pending dan history diterapkan dalam satu transaksi dengan advisory lock; `up` berulang tidak mem-posting ulang. Default deadline 2m; ubah sebelum subcommand: `go run ./cmd/migrate -timeout 5m up`. Jangan mengedit SQL yang sudah diterapkan.

Upgrade skema lama terverifikasi pada fixture `init.sql`, varian tanpa kolom waktu, dan instalasi parsial dalam PostgreSQL 16. **Sebelum upgrade data nyata, hentikan penulisan, backup dan verifikasi restore pada DB baru**, lalu jalankan migration pada salinan lebih dahulu. Data negatif, tipe ledger asing, username duplikat, timestamp wajib NULL atau tipe monetary selain integer menggagalkan upgrade tanpa memperbaiki/menghapus data. Lihat [panduan database](docs/database.md) untuk preflight, indeks, batas kompatibilitas, backup dan rollback aman. Database operasional belum diperiksa/diubah hari 04.

`migrations/init.sql` digantikan runner. Dua utilitas DDL lama dinonaktifkan; setup-kasir deprecated hanya bekerja pada tabel legacy/nonproduction dan tidak boleh dipakai untuk bootstrap server baru. Fresh schema mendukung order domain; bootstrap owner tersedia; import data tetap pekerjaan terpisah. Kontrak baru memakai customer_id/items, bukan total_biaya/nama_customer bebas; detail, receipt dan report membutuhkan staf/role yang sesuai. Lihat [api.md](docs/api.md) sebelum memakai client lama.

Server pengembangan setelah DB tersedia:

```powershell
go run ./cmd/server
```

Buka `http://localhost:8080/login.html` sesuai PUBLIC_BASE_URL/origin setelah schema dan akun staf domain tersedia. Konfigurasi divalidasi sekali; perubahan memerlukan restart. Aset default relatif terhadap cwd repository. Shutdown menguras request sebelum menutup DB; seluruh query bisnis memakai request context. Detail/struk privat; login/daftar memakai cookie dan daftar dirender lewat DOM aman. Import akun/data legacy, UI POS lengkap dan acceptance keamanan/operasional tetap diperlukan sebelum penggunaan nyata. Jangan mengekspos deployment publik dari increment ini.

## Perintah verifikasi

| Kebutuhan | Perintah/status saat ini |
|---|---|
| Format area yang direfactor | `gofmt -w cmd internal` (cek saja: `gofmt -l cmd internal`) |
| Build seluruh package | `go build ./...` — konflik main terselesaikan; tidak menjalankan utilitas |
| Test wiring terarah tanpa PostgreSQL | `go test ./internal/http/... ./internal/app -count=1 -timeout=45s` |
| Config, DB initialization dan wiring yang berubah hari 03 | `go test ./internal/config ./internal/platform ./internal/http ./internal/http/middleware ./internal/app -count=1 -timeout=45s` |
| Use case unit/fake repository | Tanpa env test: `go test ./internal/service -count=1 -timeout=45s` — DB integration di-skip; clock/ID/token sintetis, transaction/rollback intent |
| Satu alur transaction service PostgreSQL | Isi ULAUNDRY_TEST_ADMIN_DSN pada cluster test khusus, lalu `go test ./internal/service -run '^TestPostgresPaymentUseCaseAtomicity$' -count=1 -timeout=90s -v` — upgrade 3→4, rollback payment-ledger-audit, retry/replay |
| Domain: transisi, uang, validasi dan edge cases | `go test ./internal/domain -count=1 -timeout=45s` — tanpa DB/HTTP/.env; analisis terarah `go vet ./internal/domain` |
| PostgreSQL repository terarah | Isi `ULAUNDRY_TEST_ADMIN_DSN` seperti migration test, lalu `go test ./internal/repository/... -count=1 -timeout=120s -v`; membuat/menghapus DB `ulaundry_repository_test_*`; tidak membaca `.env` |
| HTTP/middleware/wiring hari 08 | `go test ./internal/http/... ./internal/app -count=1 -timeout=45s` — success/error contract, malformed/oversized/duplicate JSON, query/path, 404/405/HEAD, CORS/JWT, panic redaction, lifecycle; tanpa DB |
| Autentikasi/session hari 09 | `go test ./internal/security ./internal/http/... ./internal/config ./internal/platform ./internal/app -count=1 -timeout=60s` — claims, cookie/CSRF/logout, rate limit dan startup; integration auth opt-in: lihat docs/security.md |
| Kompatibilitas pesan login | `node --test scripts/frontend-contract.test.cjs` — 8 test JavaScript/DOM sintetis, Node.js 18+; bukan browser/visual acceptance |
| Runner/config migration unit | `go test ./internal/config ./internal/migrate ./cmd/migrate ./migrations -count=1 -timeout=45s` (integration di-skip jika env test tidak diisi) |
| PostgreSQL migration fresh/upgrade | Isi `ULAUNDRY_TEST_ADMIN_DSN` ke database `postgres` pada **cluster test khusus**, lalu `go test ./internal/migrate -run '^TestPostgresMigrations$' -count=1 -timeout=120s -v`; membuat/menghapus DB test acak, tidak membaca `.env`; lihat [isolasi test](docs/database.md#verifikasi-terarah) |
| Static analysis HTTP/app | `go vet ./internal/http/... ./internal/app` — lulus hari 08; `go vet ./...` belum menjadi bukti seluruh repository |
| Race | Belum dijalankan; dipakai pada package concurrency yang relevan saat diperlukan |
| Focused HTTP smoke | Test router memeriksa route existing dan session/logout, auth boundary, fallback API dan aset HTML; bukan acceptance bisnis dengan DB |

Pada sandbox Windows yang membatasi penulisan cache di luar workspace, set environment berikut dalam sesi PowerShell sebelum command Go:

```powershell
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
$env:GOCACHE = 'F:\project\ulaundry\.cache\go-build'
New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null
```

Path cache tersebut di luar repository `dev`; sesuaikan dengan workspace lokal. `GOPROXY=off` hanya untuk verifikasi dengan dependency yang sudah tersedia, bukan instruksi instalasi dependency baru. Unit test config memakai map sintetis; test pool/lifecycle memakai driver fake/loopback, tanpa `.env` atau PostgreSQL operasional. Hari 04 membuktikan fresh/upgrade fixture, constraint dan protokol migration pada PostgreSQL 16 terisolasi; tidak membuktikan idempotensi ledger aplikasi, skema operasional aktual, TLS production atau kesiapan produksi.

## Deployment

Target: Ubuntu, binary Go + systemd, reverse proxy, dan Cloudflare Tunnel. Repository belum memiliki konfigurasi deployment, rollback, backup/restore teruji atau CI. Kesiapan deployment publik belum dibuktikan; penuhi kriteria penerimaan sebelum rilis. Palet biru dan identitas ULaundry dipertahankan.

## Akun awal dan pengelolaan staf

Tidak ada username/password bawaan atau seed akun otomatis. Command bootstrap hanya membuat **owner pertama ketika tabel domain staff benar-benar kosong**, termasuk tidak adanya akun nonaktif. Jika sudah ada staf, command menolak; bukan jalur reset/recovery atau pengganti import akun legacy. Dua bootstrap serentak hanya menghasilkan satu akun. Tidak menghapus/mengubah kasir lama atau data bisnis.

Dari PowerShell pada instalasi yang konfigurasi DB-nya sudah benar, setelah backup dan restore salinan diverifikasi untuk data existing:

```powershell
Set-Location F:\project\ulaundry\dev
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/admin bootstrap-owner -username owner -name "Nama Owner"
go run ./cmd/server
```

Password dan konfirmasi diminta melalui terminal **tanpa echo** menggunakan [x/term.ReadPassword](https://pkg.go.dev/golang.org/x/term#ReadPassword), lalu di-hash sesuai policy hari 09. Pilih password baru; jangan memakai fixture test. Tidak ada flag password plaintext, password di stdout, file credential sementara, atau akun hardcoded. Command memvalidasi password sebelum membuka DB dan menggunakan LoadDatabase dengan policy TLS/credential production yang sama; tidak memerlukan JWT/cookie secret hanya untuk bootstrap. Migration harus lengkap terlebih dahulu. Mode interaktif memerlukan terminal nyata. x/term v0.47.0 (dan x/sys transitive) ditambahkan untuk pembacaan terminal aman lintas Windows/Ubuntu, bukan framework aplikasi.

Untuk otomasi terkontrol, `-password-stdin` menerima satu baris dari pipe privat dan menolak terminal yang menampilkan input. Jangan menulis password literal dalam argumen/history/env, file sementara, atau transcript. Contoh Ubuntu dengan prompt systemd yang menyalurkan hasil langsung ke stdin binary admin:

```bash
systemd-ask-password 'Password owner baru ULaundry' | ./ulaundry-admin bootstrap-owner -username owner -name 'Nama Owner' -password-stdin
```

Jangan gunakan command itu sebagai recovery akun existing. Pada kegagalan dengan hasil jaringan tak pasti, periksa keberadaan akun sebelum mencoba lagi; bootstrap ulang tetap ditolak bila akun pertama sudah commit. Command tidak otomatis menjalankan migration, mengaktifkan server, atau mereset credential.

Setelah server dijalankan manual, masuk melalui `http://localhost:8080/login.html` sesuai PUBLIC_BASE_URL. Owner membuka `/staff.html` dari navigasi antrean untuk membuat owner/admin/cashier lain, menonaktifkan/mengaktifkan, mengganti role atau mereset password. Admin/cashier tidak dapat mengelola staf meskipun mengirim request langsung. Reset sendiri mengakhiri sesi sendiri dan memerlukan login dengan password baru. Owner tidak dapat menonaktifkan atau menurunkan role sendiri; pengalihan owner dilakukan dari owner aktif lain. Password lama/hash tidak tersedia di UI/API.

Lihat [permission matrix dan kontrak akun](docs/api.md#permission-matrix-hari-10). Konfigurasi aplikasi hanya punya policy owner saat ini; tidak membuat modul konfigurasi baru. Audit penolakan penting disimpan terpisah dari transaksi bisnis yang rollback. Migration 000006 menambah version staf, allowed audit actions dan permission denial tanpa mengganti akun/hash existing; SQL 000001–000005 tetap immutable.

Verifikasi terarah hari 10 (tidak membaca .env/test DB operasional):

```powershell
go test ./internal/service -skip '^TestPostgres' -count=1 -timeout=60s
go test ./internal/http/... ./cmd/admin ./internal/migrate -skip '^TestPostgres' -count=1 -timeout=60s
node --test scripts/frontend-contract.test.cjs
go build ./...
```

Integration khusus constraint user/role dan transaction terkait (cluster test khusus, role test dengan CREATEDB, target postgres untuk membuat/menghapus DB disposable):

```powershell
$env:ULAUNDRY_TEST_ADMIN_DSN = 'postgres://TEST_ADMIN@127.0.0.1:55439/postgres?sslmode=disable'
go test ./internal/service -run '^TestPostgresStaffAdministration$' -count=1 -timeout=90s -v
```

Bukti hari 10: permission/negative paths/rollback dan stdin command lulus; 8 test JavaScript lulus; PostgreSQL 16 membuktikan bootstrap race, upgrade 5→6 dengan akun/hash existing sintetis tetap, username/role constraints, denial audit pada pool satu koneksi, version conflict, revocation dan audit rollback. Native build/vet dan cross-build command admin Linux amd64 lulus. Prompt TTY nyata Windows/Ubuntu, browser/mobile visual dan deployment produksi belum diuji; dependency ReadPassword telah ditinjau dan jalur stdin diuji tanpa credential nyata. Tidak menjalankan full suite atau bootstrap pada akun operasional.

## Pelanggan dan tracking — hari 11

- Customer management — buka `/customers.html` setelah login untuk nama, kontak/alamat/catatan opsional, pencarian, edit dengan version, riwayat order, struk internal dan rotasi tautan tracking. Tidak ada field identitas tambahan yang diwajibkan.
- Privasi — `/tracking.html#TOKEN` hanya menampilkan kode/status/estimasi/pesan; token crypto/rand 256 bit di-hash SHA-256 pada DB, unik, berlaku 90 hari dan dapat diganti staf. ID order tidak menjadi credential publik. Struk tetap privat. Baca [privacy.md](docs/privacy.md) dan [API](docs/api.md).
- Migration — jalankan prosedur migrator existing sampai 000007 pada deployment yang terkontrol; tidak ada import/merge/delete data operasional otomatis. Token existing mendapat grace 90 hari sejak migration. Writer schema 6 tidak kompatibel; rollback memakai forward fix, bukan menghapus kolom/audit/expiry.

Verifikasi terarah (dari root repo, tidak menjalankan full suite):

```powershell
# Gunakan cache workspace bila cache Go profil Windows tidak dapat ditulis.
$env:GOCACHE = 'F:\project\ulaundry\.cache\go-build'
go test ./internal/domain ./internal/platform ./internal/service ./internal/http/... ./internal/migrate -run 'TestTracking|TestCustomer|TestCreateOrder|TestServiceFailuresAreRedactedAndReceiptIsEscaped|TestExistingRoutes|TestCatalog$' -count=1 -timeout=60s
node --test scripts/frontend-contract.test.cjs
# Hanya cluster test khusus; helper membuat/menghapus DB disposable.
$env:ULAUNDRY_TEST_ADMIN_DSN = 'postgres://TEST_ADMIN@127.0.0.1:55441/postgres?sslmode=disable'
go test ./internal/service ./internal/repository/postgres -run '^TestPostgres(CustomerPrivacy|TrackingRepositoryConstraints)$' -count=1 -timeout=90s -v
go build ./...
```

Batas bukti: UI memakai test DOM sintetis; browser Android/printer/konfigurasi proxy produksi belum diverifikasi. Jangan mencatat path token pada proxy/Cloudflare/analytics. Lihat [laporan hari 11](docs/progress/day-11.md).

## Layanan dan harga hari 12 — Tindakan — Hasil

- Pengelolaan — `/services.html` tersedia dari antrean untuk owner/admin: kg/item, harga rupiah, durasi menit, status aktif, urutan dan histori. Harga harus eksplisit; Rp0 sah. Cashier membaca daftar aktif melalui API. Semua total order dihitung server dan disimpan sebagai snapshot.
- Schema — migrator existing sampai 000008 menambah display_order dan service_revisions append-only, termasuk baseline nilai existing. Tidak ada data contoh dalam migration. Backup/restore salinan dan penghentian writer tetap diperlukan sebelum deployment; rollback memakai binary yang menulis histori versi 8 atau forward fix.
- Demo — pada database disposable kosong berakhiran `_demo` yang sudah dimigrasi: `psql -v ON_ERROR_STOP=1 -d ulaundry_demo -f scripts/catalog-demo.sql` (atur koneksi ke cluster pengembangan). Hanya tiga layanan sintetis dan revisi; tanpa akun atau data pribadi. Pemanggilan ulang ditolak, bukan overwrite. Bootstrap owner tetap command terpisah.
- Verifikasi terarah — command berikut tidak menjalankan test auth; bukti dan batas verifikasi di [laporan hari 12](docs/progress/day-12.md).

```powershell
$env:GOCACHE = 'F:\project\ulaundry\.cache\go-build'
go test ./internal/domain ./internal/service ./internal/http/handler ./internal/migrate -run 'TestCatalog|TestLineCalculation|TestQuantityAndMoneyFailures|TestWeightAgainstExactArithmetic|TestOrderTotalsAndSnapshots|TestOrderValidation|TestCreateOrder' -count=1 -timeout=60s
node --test --test-name-pattern=catalog scripts/frontend-contract.test.cjs
# Hanya cluster test khusus; helper membuat/menghapus database disposable.
$env:ULAUNDRY_TEST_ADMIN_DSN = 'postgres://TEST_ADMIN@127.0.0.1:55443/postgres?sslmode=disable'
go test ./internal/service -run '^TestPostgresCatalog' -count=1 -timeout=120s -v
go vet ./internal/domain ./internal/service ./internal/repository/postgres ./internal/http/dto ./internal/http/handler
go build ./cmd/server ./cmd/migrate
```
