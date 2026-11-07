# ULaundry POS & Management System

ULaundry adalah aplikasi operasional laundry di Perumahan Viola milik Obed Erwin Manambe. Stack: Go (`net/http`), PostgreSQL, HTML, CSS, dan JavaScript tanpa framework frontend besar.

Status per 7 November 2026: **konfigurasi/lifecycle hari 03, belum layak produksi**. Lihat [audit baseline](docs/audit-baseline.md), [arsitektur target dan status perpindahan](docs/target-architecture.md), [kriteria penerimaan](docs/acceptance-criteria.md), [backlog 30 hari](docs/backlog-30-days.md), dan [laporan hari 03](docs/progress/day-03.md).

## Implementasi saat ini

- Login kasir dengan hash bcrypt dan JWT; hardening session dan role belum lengkap.
- API daftar/detail, pembuatan dan perubahan status pesanan. Pembuatan belum cocok dengan skema fresh install; filter status belum memfilter.
- Pencatatan kas ketika status menjadi `Sudah Lunas`, dalam transaksi database; belum aman dari posting ganda.
- Struk HTML dengan escaping data dinamis dan dialog cetak browser; printer fisik 58/80mm belum diverifikasi.
- Halaman login dan daftar antrean; belum tersedia UI POS lengkap, pembayaran, shift, dan laporan.
- `/api/health` memeriksa database; belum ada pemisahan liveness/readiness.

Struktur sekarang: `cmd/server/` bootstrap, `internal/app/` lifecycle, `internal/config/` konfigurasi bertipe tervalidasi, `internal/platform/` pool PostgreSQL, `internal/http/` router/handler/middleware, `models/` DTO, dan `frontend/` aset statis. Constructor menginjeksi DB/config; SQL/aturan bisnis tetap di handler sebagai transisi menuju service/repository. Semua executable memakai DB dependency lokal; adapter global `config/database.go` sudah dihapus. `migrations/init.sql` belum memiliki pencatatan versi.

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

Isi koneksi DB pengembangan sesuai database terisolasi, lalu isi `JWT_SECRET` dan `COOKIE_SECRET` dengan dua secret acak independen minimal **32 byte** masing-masing (misalnya 32 byte dari generator kriptografis yang dikodekan Base64). Tidak ada secret bawaan; nilai kosong, pendek, placeholder umum dan karakter berulang ditolak. Jangan gunakan secret contoh/test dalam deployment. Konfigurasi cookie sudah divalidasi, namun login saat ini masih JWT Bearer/localStorage; cookie HttpOnly/session/CSRF belum diimplementasikan (hari 09).

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

CORS sekarang memeriksa origin request persis terhadap allowlist dan memakai `Vary: Origin`; origin asing ditolak 403, tanpa wildcard fallback. Preflight dengan origin/method/header diizinkan mendapat 204 sebelum auth/router; method yang diizinkan GET/POST/PUT, header Content-Type/Authorization. Request tanpa Origin tetap mengikuti autentikasi route; CORS tidak melindungi detail/struk publik dari akses langsung.

Skema awal hanya untuk database pengembangan yang telah disiapkan (contoh `ulaundry_dev`):

```powershell
psql -h 127.0.0.1 -U postgres -d ulaundry_dev -v ON_ERROR_STOP=1 -f migrations/init.sql
```

Sesuaikan `DB_NAME` dengan database tersebut. Ini **bukan migration runner berversi** dan kompatibilitas upgrade belum dibuktikan. Jangan menjalankan utilitas lama pada data operasional: `cmd/legacy/setup-kasir/` masih memakai akun/password tetap (sudah tidak mencetak password); `cmd/legacy/setup-keuangan/` menyisipkan pengeluaran contoh setiap dipanggil; `cmd/legacy/upgrade-pesanan/` masih DDL informal. Ketiganya ditolak pada mode production dan tidak dipanggil saat server startup. Bootstrap owner aman masih pekerjaan hari 10. Fresh install belum menyediakan jalur akun awal aman dan create order akan gagal terhadap skema ini; rincian ada di audit.

Server pengembangan setelah DB tersedia:

```powershell
go run ./cmd/server
```

Buka `http://localhost:8080/login.html` sesuai PUBLIC_BASE_URL/origin yang diizinkan. Konfigurasi dibaca/divalidasi sekali; perubahan environment memerlukan restart. Aset default relatif terhadap cwd root repository; ubah STATIC_DIR sesuai packaging jika diperlukan. Shutdown normal menguras request sebelum menutup DB dan melaporkan error close. Timeout HTTP tidak menghentikan otomatis query handler lama yang belum context-aware. Gunakan lingkungan pengembangan terbatas; detail/struk publik, XSS dan risiko keuangan masih terbuka.

## Perintah verifikasi

| Kebutuhan | Perintah/status saat ini |
|---|---|
| Format area yang direfactor | `gofmt -w cmd internal` (cek saja: `gofmt -l cmd internal`) |
| Build seluruh package | `go build ./...` — konflik main terselesaikan; tidak menjalankan utilitas |
| Test wiring terarah tanpa PostgreSQL | `go test ./internal/http/... ./internal/app -count=1 -timeout=45s` |
| Config, DB initialization dan wiring yang berubah hari 03 | `go test ./internal/config ./internal/platform ./internal/http ./internal/http/middleware ./internal/app -count=1 -timeout=45s` |
| Test handler yang dipindahkan | `go test ./internal/http/handler -run '^TestGetSemuaPesanan'` — connector SQL sintetis, tanpa `.env`/DB nyata |
| PostgreSQL integration / migration | Belum dijalankan hari 03; ping/pool/cleanup diuji dengan connector SQL sintetis. TLS/driver PostgreSQL nyata belum diverifikasi; migration/repository integration wajib DB terisolasi pada hari terkait |
| Static analysis | `go vet ./...` tersedia sebagai command; belum dijalankan pada hari 02 |
| Race | Belum dijalankan; dipakai pada package concurrency yang relevan saat diperlukan |
| Focused HTTP smoke | Test router di atas memeriksa sembilan URL lama, auth boundary dan aset HTML; bukan acceptance bisnis dengan DB |

Pada sandbox Windows yang membatasi penulisan cache di luar workspace, set environment berikut dalam sesi PowerShell sebelum command Go:

```powershell
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
$env:GOCACHE = 'F:\project\ulaundry\.cache\go-build'
New-Item -ItemType Directory -Path $env:GOCACHE -Force | Out-Null
```

Path cache tersebut di luar repository `dev`; sesuaikan dengan workspace lokal. `GOPROXY=off` hanya untuk verifikasi dengan dependency yang sudah tersedia, bukan instruksi instalasi dependency baru. Unit test config memakai map sintetis; test pool/lifecycle memakai driver fake/loopback, tanpa `.env` atau PostgreSQL operasional. Keberhasilan ini tidak membuktikan ledger, schema upgrade, atau kesiapan produksi.

## Deployment

Target: Ubuntu, binary Go + systemd, reverse proxy, dan Cloudflare Tunnel. Repository belum memiliki konfigurasi deployment, rollback, backup/restore teruji atau CI. Kesiapan deployment publik belum dibuktikan; penuhi kriteria penerimaan sebelum rilis. Palet biru dan identitas ULaundry dipertahankan.
