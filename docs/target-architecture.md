# Arsitektur Target Bertahap ULaundry

Acuan: audit hari 01 pada commit `94ffc77`, diperbarui hari 02. Bagian target tetap rencana; status yang sudah diimplementasikan dipisahkan di bawah. Tetap Go/PostgreSQL, net/http, frontend sederhana dan palet ULaundry; tanpa framework DI atau frontend besar.

## Struktur yang sudah tersedia — Hari 02

```text
cmd/server/main.go                   load config, connect DB, signal context
internal/app/app.go                  compose dependencies, listen/serve/shutdown/close
internal/config/config.go            snapshot environment dan default lama
internal/platform/database.go        sql.Open + Ping dengan kebijakan koneksi lama
internal/http/router.go              sembilan route API dan static FileServer
internal/http/handler/               struct Handler dengan DB/secret injection
internal/http/middleware/            struct Middleware dengan secret/origin injection
cmd/legacy/{setup-kasir,setup-keuangan,upgrade-pesanan}/main.go
config/database.go                  adapter DB global khusus utilitas legacy
models/, frontend/, migrations/      tetap seperti baseline
```

`cmd/server` adalah satu bootstrap aktif, tanpa shim `cmd/main.go`. Router menerima kontrak Endpoints dan filesystem; handler menerima interface Database dengan operasi SQL lama, bukan repository domain. Middleware menerima snapshot secret/origin, tanpa membaca environment per request. App memiliki DB dan menutupnya setelah HTTP berhenti; listener failure, serve failure, grace timeout dan DB-close error dikembalikan ke executable. Setelah grace timeout, koneksi HTTP ditutup paksa; query SQL lama belum context-aware dan tidak dijamin berhenti saat klien putus.

Perilaku bisnis, SQL, status, nominal, response, URL/metode/auth boundary dan file frontend dipertahankan. Timeout/port/cwd/default origin lama juga dipertahankan. Ini belum menutup risiko privacy, JWT, wildcard CORS/preflight, create order/schema atau ledger dari audit. Snapshot konfigurasi belum tervalidasi lengkap; restart diperlukan saat environment berubah.

Test handler lama diganti connector database/sql sintetis: tanpa init, perubahan cwd, `.env`, network DB atau log data pelanggan. Router diuji lewat fake endpoints dan file HTML asli; lifecycle memakai listener loopback/DB fake untuk membuktikan drain/close/error propagation. Tidak menjalankan utilitas atau PostgreSQL integration.

## Kompatibilitas sementara dan penghapusan berikutnya

| Jalur/perintah lama | Pengganti hari 02 | Sisa pekerjaan |
|---|---|---|
| `go run ./cmd/main.go` | `go run ./cmd/server` | File lama sudah dihapus; tidak ada bootstrap paralel |
| Tiga file utilitas di `cmd/` | Direktori executable terpisah di `cmd/legacy/` | Tetap tidak aman untuk data nyata; DDL/demo diganti migration hari 04, bootstrap akun hari 10 |
| Fungsi package `handlers` | Method `Handler` di `internal/http/handler` dan `Middleware` internal | Package lama sudah dipindah; kompatibilitas yang dijaga adalah HTTP, bukan API Go internal |
| `config/database.go` | Server langsung memakai config/platform internal | **Hapus adapter ini hari 03 setelah ketiga utilitas menerima dependency/config baru**; jangan hapus sebelum import legacy dialihkan |
| SQL dan DTO di handler/model | Sementara tetap, memakai Database interface | Domain/repository/service/DTO dipisah hari 05–08, bukan sekaligus pada wiring |

Hari 03 memperluas config tervalidasi, SSL/pool/deadline dan lifecycle DB tanpa global. Hari 04 baru mengganti DDL informal dengan migration berversi; jangan menghapus legacy command sebelum pengganti berfungsi atau ditandai tidak boleh dipakai.

## Batas tanggung jawab

```text
cmd/server (composition root: config, DB, router, lifecycle)
  -> internal/http/handler + middleware + DTO
  -> internal/service (otorisasi use case, aturan alur, transaction boundary)
  -> internal/repository (kontrak sesuai kebutuhan use case)
  -> internal/repository/postgres (SQL, lock, constraint/error mapping)
  -> PostgreSQL

internal/domain: entitas, uang, quantity, status/transisi, error bisnis
internal/config: konfigurasi bertipe dan validasi sekali saat startup
internal/platform: DB, logger, clock dan generator ID/token
web/: HTML, CSS, JavaScript, template, manifest/icon seperlunya
migrations/: SQL berurutan + history versi; fixture terpisah
cmd/admin/: bootstrap/migrasi terkontrol, bukan saat server startup
docs/ dan scripts/: kontrak, operasi dan helper yang diperlukan
```

Panah adalah alur pemanggilan. Dependency kode service mengarah ke domain/kontrak repository, bukan adapter PostgreSQL. Domain tidak mengimport HTTP/DB; handler tidak menyusun SQL. Interface kecil dibentuk dari kebutuhan pemakai, bukan CRUD generik semua tabel. Composition root menginjeksi adapter melalui constructor/struct biasa. Hindari package kosong untuk memenuhi diagram.

Alur order: HTTP memvalidasi bentuk request → service memeriksa actor, layanan aktif, harga/quantity/diskon → domain menghitung integer rupiah → repository menyimpan snapshot → response DTO. Total final ditentukan server.

Alur pembayaran: service mengendalikan satu transaksi/unit of work → lock order/payment state atau cek version → unique idempotency key → payment, cash ledger dan audit → commit diperiksa → response. Retry key yang sama memberi hasil sama; payload berbeda pada key sama ditolak. Lock tidak menggantikan idempotensi serial; status laundry tidak menjadi bukti pembayaran. Refund merupakan transaksi baru terotorisasi, bukan penghapusan ledger.

Tracking publik memakai lookup token kuat dan DTO minimal, terpisah dari detail internal. Auth context membawa identitas/role ke service, bukan sekadar token valid. Session same-origin diarahkan ke cookie HttpOnly/Secure/SameSite dengan CSRF guard sesuai pada hari 09; keputusan dan migrasi UI harus didokumentasikan.

## Tahap dan syarat keluar

| Tahap | Hari | Hasil dan syarat keluar |
|---|---|---|
| A — Bootstrap | 02 | Executable terpisah, router/lifecycle/constructor, URL dan perilaku bisnis tetap; build package berhasil. Isolasi init test lama sebelum mencoba test; SQL tidak harus dipindahkan sekaligus. |
| B — Config/skema | 03–04 | Config tervalidasi, pool/context/close, env contoh, migration berversi dan fresh/upgrade evidence. Inventaris DB lama secara terotorisasi sebelum adoption baseline. |
| C — Aturan/boundary | 05–08 | Domain, repository, service dan handler tipis per use case; filter benar, DTO/error contract, SQL/error tidak bocor. |
| D — Operasi inti | 09–17 | Session/roles/customer/tracking/layanan/order/payment/shift/report/audit bertahap; concurrency/uang dibuktikan pada DB terisolasi. |
| E — Antarmuka | 18–23 | Aset web teratur, warna lama menjadi tokens; workflow staf, cetak, Android, accessibility dan PWA terverifikasi terarah. |
| F — Operasi/rilis | 24–30 | Hardening, health/log, Ubuntu/proxy/tunnel, restore, CI dan acceptance; rilis mengikuti bukti gate dan otorisasi deployment. |

Critical/High terbuka tetap menghalangi rilis sepanjang tahap awal. Urutan harian bukan izin mengoperasikan baseline tidak aman. Kebutuhan pemakaian nyata lebih awal memerlukan review prioritas/scope, bukan rewrite terselubung.

## Peta perpindahan file baseline ke target

Jalur relatif terhadap root `dev`; kolom pertama mengacu snapshot hari 01, bukan daftar file aktif hari 02. Status perpindahan yang sudah selesai dijelaskan di atas. Pemecahan berdasarkan tanggung jawab, bukan sekadar memindah nama file.

| File sekarang | Tujuan | Hari / catatan |
|---|---|---|
| `cmd/main.go` | `cmd/server/main.go`, `internal/http/router.go`, lifecycle platform | 02/03/25: composition root saja; route/aset tetap sampai perubahan privacy disengaja |
| `cmd/setup_kasir.go` | executable terpisah sementara, lalu bootstrap di `cmd/admin` | 02/10: jangan salin password tetap sebagai default baru; hapus jalur lama sesudah pengganti aman |
| `cmd/setup_keuangan.go` | DDL ke `migrations/`, contoh kas ke fixture/demo terpisah | 02/04: jangan menjalankan helper pada DB nyata; hilangkan contoh otomatis saat pengganti siap |
| `cmd/upgrade_pesanan.go` | migration terurut | 02/04: cegah DDL informal setelah adopsi; tidak menghapus data lama |
| `config/database.go` | `internal/config/config.go`, `internal/platform/database.go` | 03: return DB/error, tanpa global/log.Fatal di library; close di lifecycle |
| `models/pesanan.go` | `internal/domain/order.go`, `internal/http/dto/order.go` | 05/08/12: pisahkan status/waktu bertipe, DTO kompatibel sementara atau perubahan kontrak terdokumentasi |
| `models/kasir.go` | `internal/domain/user.go`, DTO login | 05/09/10: request password/hash tidak masuk response |
| `handlers/pesanan.go` create/list/filter/detail | handler order, service order, kontrak repository dan adapter postgres order | 06–08/12/13: pindah endpoint bertahap, URL staf dipertahankan bila aman |
| `handlers/pesanan.go` status/DEBIT | service order/payment dan adapter transaksi postgres | 05/07/13/14: payment terpisah; satu boundary payment/ledger/audit |
| `handlers/pesanan.go` tracking/struk | handler tracking/receipt, DTO minimal, `web/templates/receipt.html` bila diperlukan | 11/21: batasi/hentikan public ID lama; jangan pertahankan kebocoran demi kompatibilitas |
| `handlers/auth.go` | handler auth, service auth, repository user | 08–10: error/session contract diperbarui bersama UI |
| `handlers/middleware.go` | `internal/http/middleware/` | 02/08–10/24/25: auth, CORS, limits, headers, request ID; tidak membawa wildcard production |
| `handlers/laporan.go` | handler report, service report, query adapter postgres | 06–08/16: sumber payment/ledger, error agregasi ditangani |
| `handlers/health.go` | handler health dan readiness DB bertimeout | 02/25: liveness independen DB; deprecation `/api/health` dicatat |
| `handlers/pesanan_test.go` | HTTP test dengan fake, integration postgres opt-in | 02/06/08: hilangkan init/cwd/.env, jangan memindahkan side effect lama |
| `frontend/index.html` | `web/index.html`, aset CSS/JS bersama | 18/20–22: DOM aman, tokens warna lama, same-origin |
| `frontend/login.html` | `web/login.html` + aset bersama | 18/19: session baru, keyboard/mobile |
| `migrations/init.sql` | migration baseline/adoption bernomor dan migration lanjutan | 04: inspeksi schema lama, history, fresh/upgrade, forward recovery |
| `.env.example` | tetap root, sesuai config bertipe | 03: komentar/default aman, tanpa secret |
| `go.mod`, `go.sum` | tetap root | 02/28: pertahankan versi terbukti, tidy jika import perlu; tanpa upgrade massal |
| `README.md` | tetap root dan tautan docs spesifik | Setiap perubahan command/setup; status faktual |

Fitur baru customer, service catalog, payment, shift dan audit masuk boundary saat diperlukan. Error service bertipe validation/not found/conflict/forbidden dipetakan ke HTTP; detail driver hanya dicatat aman dengan request ID.

## Kompatibilitas dan data

- Satu route memakai satu implementasi aktif; adapter sementara dibolehkan, hindari dua aturan bisnis paralel. Hapus file lama setelah pengganti, wiring dan dokumentasinya terverifikasi.
- Hari 02 pisahkan utilitas dari server tanpa menjalankannya. Ganti command file `go run ./cmd/main.go` dengan command package terdokumentasi; jangan mempertahankan shim startup ganda.
- Hari 04 jangan menandai DB lama sebagai baseline hanya karena tabel ada. Bandingkan kolom/default/constraint, backup dan adoption plan; expand/backfill/validate terlebih dahulu. Destructive migration memerlukan persetujuan pemilik.
- `INT` → `BIGINT`, snapshot dan pemetaan status mempertahankan nilai lama. Status tak dikenal dilaporkan untuk review, bukan diubah diam-diam. Posting ganda lama direkonsiliasi terpisah, bukan dihapus otomatis.
- Rollback binary/aset harus kompatibel dengan schema maju. Untuk data-loss risk, pakai forward fix/restore teruji, bukan down otomatis.
- Packaging statis (path terkonfigurasi atau embed) diputuskan saat wiring/UI; Ubuntu harus berjalan tanpa cwd tersembunyi. Pola utama: binary + systemd non-root, reverse proxy, Cloudflare Tunnel.

## Bukti perpindahan

Hari 01 hanya dokumen dan dua build. Hari 02 memakai formatting, build package dan targeted tests router/handler/middleware/lifecycle; tidak memverifikasi query pada PostgreSQL. Hari berikut memakai unit domain, HTTP fake, DB terisolasi untuk SQL/schema/transaksi, negative auth tests, browser/manual CSS/JS, serta release checks pada gate 29–30. Command authoritative ada di README; hasil nyata dicatat di laporan harian, bukan ditafsirkan sebagai seluruh arsitektur target selesai.
