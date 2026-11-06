# ULaundry POS & Management System

ULaundry adalah aplikasi operasional laundry di Perumahan Viola milik Obed Erwin Manambe. Stack: Go (`net/http`), PostgreSQL, HTML, CSS, dan JavaScript tanpa framework frontend besar.

Status per 7 November 2026: **fondasi struktur hari 02, belum layak produksi**. Lihat [audit baseline](docs/audit-baseline.md), [arsitektur target dan status perpindahan](docs/target-architecture.md), [kriteria penerimaan](docs/acceptance-criteria.md), [backlog 30 hari](docs/backlog-30-days.md), dan [laporan hari 02](docs/progress/day-02.md).

## Implementasi saat ini

- Login kasir dengan hash bcrypt dan JWT; hardening session dan role belum lengkap.
- API daftar/detail, pembuatan dan perubahan status pesanan. Pembuatan belum cocok dengan skema fresh install; filter status belum memfilter.
- Pencatatan kas ketika status menjadi `Sudah Lunas`, dalam transaksi database; belum aman dari posting ganda.
- Struk HTML dengan escaping data dinamis dan dialog cetak browser; printer fisik 58/80mm belum diverifikasi.
- Halaman login dan daftar antrean; belum tersedia UI POS lengkap, pembayaran, shift, dan laporan.
- `/api/health` memeriksa database; belum ada pemisahan liveness/readiness.

Struktur sekarang: `cmd/server/` bootstrap, `internal/app/` lifecycle, `internal/config/` pengambilan konfigurasi, `internal/platform/` pembukaan DB, `internal/http/` router/handler/middleware, `models/` DTO, dan `frontend/` aset statis. Constructor menginjeksi DB dan konfigurasi ke handler; SQL/aturan bisnis tetap di handler sebagai tahap transisi menuju service/repository. Tiga utilitas di `cmd/legacy/` masih memakai adapter sementara `config/database.go`; server tidak menggunakan DB global. `migrations/init.sql` belum memiliki pencatatan versi.

## Prasyarat dan setup lokal

- `go.mod` menetapkan Go **1.27.3**; toolchain lokal itu berhasil membangun seluruh package pada hari 02.
- PostgreSQL lokal/VM dan database kosong khusus pengembangan, bukan database operasional.
- Jalankan dari root repository (`F:\project\ulaundry\dev` pada workspace ini).

```powershell
git clone https://github.com/obedman-gh/u-laundry.git
cd u-laundry
Copy-Item .env.example .env
```

Isi `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`, dan `JWT_SECRET` di `.env` dengan nilai lokal. Gunakan secret acak yang kuat, bukan placeholder. Tambahkan `ALLOWED_ORIGIN=http://localhost:8080`; variabel ini dibaca kode tetapi belum tercantum di contoh. `.env` diabaikan Git. Startup belum memvalidasi seluruh konfigurasi dan koneksi DB memaksa `sslmode=disable`.

Skema awal hanya untuk database pengembangan yang telah disiapkan (contoh `ulaundry_dev`):

```powershell
psql -h 127.0.0.1 -U postgres -d ulaundry_dev -v ON_ERROR_STOP=1 -f migrations/init.sql
```

Sesuaikan `DB_NAME` dengan database tersebut. Ini **bukan migration runner berversi** dan kompatibilitas upgrade belum dibuktikan. Jangan menjalankan utilitas lama pada data operasional: `cmd/legacy/setup-kasir/` memakai akun/password tetap dan mencetak password; `cmd/legacy/setup-keuangan/` menyisipkan pengeluaran contoh setiap dipanggil. `cmd/legacy/upgrade-pesanan/` masih DDL informal. Ketiganya hanya dipisah agar tidak bentrok dengan build server; tidak dijalankan saat startup. Bootstrap owner aman masih pekerjaan hari 10. Fresh install belum menyediakan jalur akun awal aman dan create order akan gagal terhadap skema ini; rincian ada di audit.

Server pengembangan setelah DB tersedia:

```powershell
go run ./cmd/server
```

Buka `http://localhost:8080/login.html`. Server menggunakan port 8080 dan aset relatif `./frontend`; cwd harus root repository. Konfigurasi dibaca sekali saat bootstrap; perubahan environment memerlukan restart. Default timeout tetap 10 detik baca/tulis, 15 detik idle, dan 5 detik shutdown. Saat shutdown normal, request dikuras sebelum DB ditutup; kegagalan shutdown/close dilaporkan. Validasi lengkap, SSL/pool/context deadline ditangani hari 03. Gunakan lingkungan pengembangan terbatas; detail/struk masih publik dan frontend memiliki risiko XSS.

## Perintah verifikasi

| Kebutuhan | Perintah/status saat ini |
|---|---|
| Format area yang direfactor | `gofmt -w cmd config internal` (cek saja: `gofmt -l cmd config internal`) |
| Build seluruh package | `go build ./...` — konflik main terselesaikan; tidak menjalankan utilitas |
| Test wiring terarah tanpa PostgreSQL | `go test ./internal/http/... ./internal/app -count=1 -timeout=45s` |
| Test handler yang dipindahkan | `go test ./internal/http/handler -run '^TestGetSemuaPesanan'` — connector SQL sintetis, tanpa `.env`/DB nyata |
| PostgreSQL integration / migration | Belum tersedia; disiapkan pada hari perubahan query/skema, wajib DB terisolasi |
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

Path cache tersebut di luar repository `dev`; sesuaikan dengan workspace lokal. `GOPROXY=off` hanya untuk verifikasi dengan dependency yang sudah tersedia, bukan instruksi instalasi dependency baru. Test hari 02 memakai fixture/loopback terisolasi, tidak memuat `.env` atau menghubungi PostgreSQL. Keberhasilan wiring tidak membuktikan ledger, schema upgrade, atau kesiapan produksi.

## Deployment

Target: Ubuntu, binary Go + systemd, reverse proxy, dan Cloudflare Tunnel. Repository belum memiliki konfigurasi deployment, rollback, backup/restore teruji atau CI. Kesiapan deployment publik belum dibuktikan; penuhi kriteria penerimaan sebelum rilis. Palet biru dan identitas ULaundry dipertahankan.
