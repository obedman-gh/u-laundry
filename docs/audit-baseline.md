# Audit Baseline ULaundry — Hari 01

Tanggal: 7 November 2026 (Asia/Makassar). Repository: `F:\project\ulaundry\dev`. Acuan kode: `94ffc77d6fcdbb3d3c7d0a5b9c072806419e9bdd`. Git status awal bersih. Referensi baris menunjuk baseline ini; README diperbaiki hari 01.

## Kesimpulan dan batas bukti

Baseline belum layak untuk uang/data pelanggan nyata atau eksposur publik. Ada jalur debit ganda, XSS, detail/struk publik, create order tidak sesuai skema, dan build seluruh package gagal. Audit berupa pemeriksaan sumber lokal dan build; bukan penetration test, pengujian DB, pemeriksaan server Ubuntu, atau sertifikasi keamanan.

Nilai `.env` tidak dibaca. Tidak mengakses database, menjalankan setup/migration, mengubah kredensial atau layanan publik. `.env` lokal ada, diabaikan Git dan tidak terlacak; pencarian riwayat path `.env` pada refs lokal tidak menghasilkan commit. Ini tidak membuktikan seluruh riwayat/artefak bebas secret. Tidak melakukan audit CVE eksternal; versi hanya dicatat dari `go.mod`/`go.sum`.

Acuan (jalur relatif dari root repository): `../AGENTS.md` dan `../PROMPTS_30_HARI_ULaundry_PRODUCTION.md`. `../docs/` berisi `Rencana Project RLaundry.pdf`; isi PDF belum diekstrak/dijadikan bukti kebutuhan. Tidak ada laporan progres dalam repository sebelum audit ini. Backlog mengikuti prompt 30 hari tertulis.

Prioritas: **Critical** = risiko langsung integritas uang atau pengambilalihan session; **High** = workflow inti gagal, akses sensitif, atau penghambat rilis; **Medium** = keandalan/operasi/UX yang perlu diselesaikan sebelum acceptance; **Low** = konsistensi/dokumentasi nonblocking. Risiko bersyarat diberi kondisi; tidak menyatakan insiden telah terjadi.

## Inventaris dan fondasi yang tersedia

Pembaruan hari 04: H08 pada sisi migration skema sudah ditangani oleh runner berversi/transaksional, history/checksum, fixture/demo terpisah serta 17 skenario PostgreSQL 16.14 fresh/upgrade test; lihat [database.md](database.md) dan [laporan hari 04](progress/day-04.md). Constraint uang/ledger/username/timestamp dan FK opsional menutup sebagian H07. Schema operasional belum diperiksa; C01/idempotensi ledger, H01/create tanpa status, bootstrap aman dan risiko security lain tetap terbuka. Referensi `init.sql` di tabel temuan berikut tetap bukti baseline historis, bukan command setup aktif.

Pembaruan hari 06: filter status legacy kini benar-benar WHERE/bind dengan request context dan iterator/scan/close handling, sehingga bagian bug filter M01 diperbaiki. Adapter/storage domain baru menambahkan transaksi/constraint/locking/idempotency yang diuji terisolasi, tetapi belum di-wire ke server; C01 pada posting uang legacy dan H01/H07 pada create aktif belum ditutup. Bukti serta batas implementasi ada di [laporan hari 06](progress/day-06.md).

Pembaruan hari 07: server telah memakai service/repository domain; handler bebas SQL/transaksi, total klien ditolak, status/payment terpisah dan audit atomik diuji pada satu alur PostgreSQL. Detail/struk kini staf-only, tracking publik memakai token, laporan memeriksa role dan error agregasi. Ini memperbaiki jalur baru terkait C01/H01/H02/H07/H10 dan sebagian H03/H05, tetapi tidak merekonsiliasi uang/data legacy atau membuktikan acceptance produksi. Auth/session/rate-limit/bootstrap, frontend XSS/localStorage, import/cutover/restore dan bukti operasional tetap terbuka. Referensi temuan di bawah adalah baseline historis, bukan daftar endpoint aktif; lihat [arsitektur](architecture.md) dan [laporan hari 07](progress/day-07.md).

| Area | Bukti baseline |
|---|---|
| Entry point | Empat `cmd/*.go`, semuanya `package main` dengan `main()` |
| HTTP/SQL | Handler mengakses `config.DB` global langsung; belum ada service/repository |
| Model | Pesanan/Kasir juga menjadi input DTO; status string bebas, uang Go `int` / SQL `INT` (sudah integer) |
| Database | Tiga tabel: pesanan, kasir, keuangan; PK dan unique username sudah ada |
| Security dasar | bcrypt, JWT login HS256, verifikasi keluarga HMAC, SQL parameter, escaping struk |
| Transaksi | Status dan insert kas dalam satu transaksi; commit diperiksa, tetapi idempotensi belum aman |
| Server | Read/write/idle timeout, SIGTERM/interrupt, `Shutdown` 5 detik |
| UI | Login dan antrean; biru `#2563eb`/`#1d4ed8`, status `#007bff`, latar netral |
| Test | Satu test handler yang bergantung DB lokal, tanpa fixture terisolasi |

## Critical

| ID | Temuan, bukti dan dampak | Tindak lanjut / bukti selesai |
|---|---|---|
| C01 | `handlers/pesanan.go:75`: SELECT status tanpa lock sebelum update memungkinkan dua transaksi sama-sama membaca belum lunas dan memposting DEBIT. Urutan serial lunas → status lain → lunas juga memposting ulang. `keuangan` tanpa payment/order FK atau unique idempotency key (`migrations/init.sql:24`). Transaksi yang ada tidak mencegah debit ganda. | Hari 05/13/14: status pembayaran terpisah, lock/version, payment ID dan unique key; buktikan retry, concurrency, rollback dan rekonsiliasi pada DB test. |
| C02 | `frontend/index.html:58` memasukkan nama, ID, layanan, status ke `innerHTML` tanpa escaping. Create/status menerima string bebas; token berada di `localStorage` (`index.html:36`, `login.html:182`). Data tersimpan berbahaya dapat mengeksekusi script pada browser staf dan membaca token. Belum dieksploitasi runtime. | Hari 09/18–21/24: session aman, DOM `textContent`, CSP sesuai; payload tampil sebagai teks dan token tidak bocor. Jangan gunakan data tak tepercaya sampai diperbaiki. |

## High

| ID | Temuan, bukti dan dampak | Tindak lanjut / bukti selesai |
|---|---|---|
| H01 | INSERT create order (`pesanan.go:59`) tidak mengisi `status`; skema (`init.sql:9`) NOT NULL tanpa default. Fresh schema menolak create meskipun klien mengirim status (field diabaikan INSERT). DB lama belum diperiksa. | Hari 04–07: selaraskan initial status/skema; uji fresh/upgrade dan create order. |
| H02 | `cmd/main.go:29–30`: detail ID dan struk publik mengungkap nama, layanan, biaya/status, waktu pada struk. ID ditentukan klien; tidak ada token tracking acak. CORS bukan otorisasi. | Hari 10/11: auth detail/struk internal, token tracking kuat dan DTO minimal; uji anonim/role/token salah/privacy. |
| H03 | `cmd/setup_kasir.go:36` memakai kredensial tetap dalam sumber dan mencetak password. Jika pernah dipakai, akun itu mungkin aktif; ON CONFLICT DO NOTHING tidak memperbarui password. Keadaan akun nyata belum diketahui. | Hari 10: bootstrap aman tanpa password tetap/output secret. Pemilik memeriksa akun aktual; rotasi hanya dengan otorisasi. Jangan jalankan helper ini. |
| H04 | Config tidak divalidasi saat startup (`config/database.go`, `auth.go:55`, `middleware.go:47`). Login memakai secret apa adanya; middleware menolak kosong baru saat request. Placeholder contoh tidak ditolak startup, tanpa mode production. | Hari 03/09: config bertipe; production menolak kosong/placeholder/lemah, uji tanpa `.env` nyata. |
| H05 | `middleware.go:55` menerima keluarga HMAC, bukan exact HS256 yang dipakai login. Tidak ada kebijakan issuer/audience, identity/role atau kewajiban expiry eksplisit pada parse. | Hari 09/10: exact algorithm/claims/session policy; negative tests wrong algorithm, missing/expired claims, wrong issuer/audience, user nonaktif. |
| H06 | `auth.go:30–41` membedakan username tak ada/password salah. Tidak ada login rate limit, logout/revocation server atau role. Semua token valid dapat mengakses laporan (`cmd/main.go:39`). | Hari 09/10/19: generic failure, rate limit dan role matrix server; negative auth/session tests. |
| H07 | Create percaya total klien; status menerima string bebas (`pesanan.go:51,78`). Tidak ada items/quantity, diskon, transisi, nonnegative constraints atau snapshot harga. SQL INT/Go int perlu batas/overflow policy. | Hari 04/05/12/13: server menghitung integer rupiah int64/BIGINT, validation/constraints/transisi; unit dan DB evidence. |
| H08 | `migrations/init.sql` hanya IF NOT EXISTS, tanpa versi/history/rollback; skema lama dapat berbeda diam-diam. `setup_keuangan.go:28` menambahkan pengeluaran contoh setiap eksekusi dan mengabaikan error. | Hari 04/14: fresh/upgrade migration, fixture terpisah, inventaris skema dan rekonsiliasi tanpa mengubah data nyata hari ini. |
| H09 | `go build ./...` gagal: empat main di direktori sama. Build file server tunggal berhasil tetapi melewati konflik, bukan memperbaikinya. | Hari 02: entry point terpisah dan build package konvensional tanpa menjalankan setup. |
| H10 | `handlers/laporan.go:13–14` mengabaikan kedua error scan agregat; DB error/overflow dapat menghasilkan nol/parsial dengan HTTP 200. Dua query tidak menjamin satu snapshot konsisten. | Hari 06/16: error handling dan sumber/snapshot laporan benar; uji failure dan angka referensi. |
| H11 | `middleware.go:16` fallback origin `*`; OPTIONS ditangani di wrapper tetapi route method-specific tidak mendaftarkan OPTIONS (`cmd/main.go`). Koreksi bukti hari 02: OPTIONS pada `/api/pesanan/ORD-123` jatuh ke catch-all FileServer dan menghasilkan 404, bukan dugaan awal 405; wrapper preflight tidak berjalan. Contoh env tidak menyebut origin. | Hari 03/08/24: same-origin aman atau allowlist eksplisit dan OPTIONS router; HTTP checks allowed/disallowed/preflight. |

## Medium

| ID | Temuan, bukti dan dampak | Tindak lanjut / bukti selesai |
|---|---|---|
| M01 | Filter (`pesanan.go:141`) memvalidasi q tetapi query tanpa WHERE/arg; semua status dikembalikan. List tanpa urutan/pagination; slice kosong menjadi JSON null, frontend memakai forEach. | Hari 06/08/21: filter parameter, pagination/order, array kosong; bukti fixture dan empty UI. |
| M02 | DB Query/Exec/Begin/Ping tanpa context, pool limit/lifetime; SSL disable tetap dan DSN dirangkai tanpa escaping komponen. DB tidak ditutup saat shutdown; bind `:8080`, aset tergantung cwd. HTTP timeout tidak membatasi query DB. | Hari 02/03/06/25/26: config, lifecycle, deadline dan packaging; startup/failure/shutdown evidence. |
| M03 | Tanpa body limit/content-type guard, security headers, recovery aplikasi atau error contract. `http.Error` teks; login mencoba JSON lalu dapat menampilkan `[object Object]` (`login.html:179–185`). | Hari 08/19/24/25: request safety/error contract; uji invalid/oversized/content type/4xx/5xx. |
| M04 | Loop list tanpa rows.Err; lookup menyamakan seluruh DB error dengan 404; deferred rollback tak diperiksa. Banyak write/encode error diabaikan. Dapat menyamarkan kegagalan/hasil parsial. | Hari 06–08: mapping error, rows/commit/rollback handling dan pencatatan aman. |
| M05 | `pesanan_test.go:14` init mengubah cwd/load `.env`/ConnectDB, mengabaikan error awal. Satu test langsung hanya cek status/content type, tidak auth/router; URL detail dipakai untuk handler list, tanpa fixture/cleanup, mencetak seluruh body (risiko data pribadi). | Hari 02/06/08: unit/integration opt-in terisolasi tanpa side effect init atau data pribadi; jangan jalankan test baseline. |
| M06 | UI baru login/daftar, belum POS/customer/services/status/payment/shift/report/audit/PWA. Fetch daftar sekali, bukan real-time. Tanpa logout/offline/empty memadai; HTTP non-401 tak dicek, setelah redirect 401 masih parse JSON. | Hari 11–23: delivery workflow dan acceptance desktop/Android; role server otoritatif. |
| M07 | Login box width 100% + padding tanpa border-box berpotensi overflow mobile. Toggle password span klik, tanpa keyboard/semantik tombol; error tanpa pengumuman aksesibel. UI masih slang. Belum diuji browser. | Hari 18/19: tokens warna lama, semantic controls/focus, mobile/keyboard checks. |
| M08 | Health memakai DB Ping tanpa timeout, tanpa liveness terpisah. Log startup belum structured request logging. Tidak ada audit trail, systemd/proxy/tunnel config, backup/restore terverifikasi, runbook atau CI dalam repo. | Hari 17/25–28: operasi, restore drill dan gates; infrastruktur eksternal belum diperiksa. |

## Low

| ID | Temuan, bukti dan dampak | Tindak lanjut / bukti selesai |
|---|---|---|
| L01 | README baseline terpotong dengan fence tak ditutup, git clone memakai link Markdown dan Go 1.20+ berlawanan go.mod 1.27.3. Klaim Clean Architecture, tracking aman/estimasi, ledger aman, real-time, printer 58/80mm dan Tunnel ready belum didukung penuh. | Diperbaiki hari 01 pada README: status faktual, command/prasyarat benar, batas setup/build dan link audit; tidak mengklaim setup end-to-end berhasil. |
| L02 | Direct imports ditandai indirect, komentar tutorial/slang, nomor kontak contoh pada struk tanpa konfigurasi identitas usaha. | Hari 02/18/21/29: rapikan setelah wiring stabil dan identitas disepakati; palet tetap. |

## Commit terbaru dan batas klaim

| Commit | Isi / kesesuaian |
|---|---|
| `94ffc77` — 7 Nov 2026 | Pesan menyebut unit/integration test, JWT algorithm, timeout, shutdown, logging, health, CORS. Bukti: satu test DB-dependent; algoritma keluarga HMAC; timeout/shutdown ada dengan batas di atas; logging startup; health hanya DB; wildcard CORS fallback. |
| `9d65025` — 6 Nov 2026 | Skema awal/env; belum migration berversi. |
| `27b0c02`, `f05f572` — 6 Nov 2026 | Env/README; README baseline tidak selesai. |
| `2a66803` — 4 Nov 2026 | Rilis perdana; label rilis tidak membuktikan kesiapan operasi. |

## Status tindak lanjut — Hari 03

Baseline di atas tetap merujuk snapshot hari 01. H09 ditutup oleh build hari 02; H04 memperoleh config fail-fast dan positive/negative tests hari 03; H11 memperoleh allowlist tanpa wildcard dan preflight sebelum router (valid 204, origin asing 403) dengan HTTP tests hari 03. M02 kini memiliki pool/lifetime, startup PingContext/connect timeout, DB close dan config SSL; query/Ping handler lama masih belum context-aware. M05 sudah diganti fixture deterministik tanpa init/.env/DB nyata. Adapter DB global sudah dihapus dari seluruh executable. H03 dipersempit: utility lama ditolak pada production dan tidak mencetak password, tetapi kredensial tetap masih ada untuk development sehingga bootstrap aman tetap backlog. Bukti TLS/DB production, session/auth policy lain, data/uang dan UI tetap belum lengkap; jangan menyimpulkan seluruh temuan tertutup.

## Verifikasi dan keputusan hari 01

- Membaca seluruh sumber terlacak, env contoh, README, schema/test, status/lima commit dan instruksi/prompt lokal.
- Go `go1.27.3 windows/amd64`, `GOTOOLCHAIN=local`, `GOPROXY=off`: dependency lokal cukup untuk compile.
- `go build ./...`: **FAIL**, tiga redeklarasi main terhadap `cmd/main.go`.
- `go build -o NUL ./cmd/main.go`: **PASS**, tanpa binary tersimpan; bukan bukti DB/auth/routing/printer/UI/Ubuntu.
- Tidak menjalankan automated test, static analysis, race, browser, server, migration atau integration DB. Temuan runtime belum direproduksi, kecuali kegagalan build yang benar-benar dijalankan.

Tidak mengubah kode, SQL, dependency, warna atau data. Bug filter/empty state mengubah perilaku dan membutuhkan DB/UI verification; create/status/uang/security lebih berisiko. Seluruhnya masuk [backlog](backlog-30-days.md) dan [acceptance](acceptance-criteria.md). Selesai audit tidak menutup temuan dan tidak mengizinkan rilis. Blocker Critical/High harus ditutup sebelum pemakaian nyata/eksposur publik.

## Pembaruan hari 08 — HTTP boundary

Partial implementation hari 08 yang ada dilengkapi tanpa menimpa pekerjaan hari 01–07. M03 bagian body/content type/error contract/recovery dan H11 bagian API method/static fallback kini memiliki bukti HTTP/app terarah; JSON berkode berisi request_id server, unknown/duplicate/oversized request ditolak dan path API tidak jatuh ke FileServer. Frontend login membaca error.message sebagai teks; UI daftar safe DOM, session/rate-limit, request logging dan acceptance security/operasional tetap terbuka. Seluruh detail/batas bukti ada di [laporan hari 08](progress/day-08.md); bukan penutupan semua temuan baseline.
