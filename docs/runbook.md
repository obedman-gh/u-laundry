# Runbook operasi ULaundry — Hari 25

Untuk server Go/PostgreSQL saat ini. Instruksi ini belum membuktikan deployment Ubuntu/Tunnel, restore produksi atau konfigurasi systemd tersedia. Semua pemeriksaan Hari 25 memakai fake dependency atau loopback; tidak mengakses DB/server operasional. Jangan mencetak `.env`, DSN, cookie, token, password atau data pelanggan ke tiket/log. Simpan request ID, waktu kejadian dengan zona waktu, route template, status dan error class.

## Health dan timeout

| Probe | Hasil | Tindakan |
|---|---|---|
| `GET/HEAD /api/health/live` | 200, `status=alive`, tanpa query DB | Proses melayani HTTP; DB boleh sedang gagal. Jangan memakai readiness gagal sebagai alasan restart berulang. |
| `GET/HEAD /api/health/ready` | 200 dengan `status=ok`, version dan database=connected | Ping DB berhasil pada batas timeout; belum memeriksa schema, izin semua tabel, backup, kapasitas disk atau hasil transaksi. |
| Readiness 503 `health_unavailable` | DB gagal/timeout atau dependency tidak tersedia | Periksa bagian database unavailable; liveness seharusnya tetap 200. |
| Readiness 503 `not_ready` | Server memasuki draining | Hentikan routing request baru pada proxy/load balancer yang dikonfigurasi operator; tunggu shutdown. |
| `/api/health` | Alias readiness, body sukses existing dipertahankan | Integrasi lama tetap berjalan, tetapi jangan menggunakannya sebagai liveness. |

Contoh loopback Windows, tanpa Origin/cookie, dari root repository:

```powershell
curl.exe --max-time 4 -i http://127.0.0.1:8080/api/health/live
curl.exe --max-time 4 -i http://127.0.0.1:8080/api/health/ready
```

Pada Ubuntu gunakan `curl` dengan URL bind aktual. Jangan mengubah bind menjadi publik demi probe. Respons no-store, metode lain mendapat 405. CORS tetap membatasi request yang membawa Origin. Penolakan parser HTTP sebelum middleware (misalnya header terlalu besar) dapat tidak memiliki log request/ID aplikasi.

Default: read-header 5s, read 10s, write 10s, idle 15s, shutdown 5s, MaxHeaderBytes 16 KiB dan body API 64 KiB. `HEALTH_READINESS_TIMEOUT=2s` positif dan harus lebih pendek dari `HTTP_WRITE_TIMEOUT`; memendekkan write timeout memerlukan penyesuaian readiness. Ping menggunakan context-aware PostgreSQL driver; tidak membuat goroutine probe terlepas. WriteTimeout membatasi penulisan respons, bukan pembatalan otomatis setiap query bisnis. Request cancellation/forced connection close diteruskan ke operasi DB berbasis context. Connect/ping startup dan pool memakai konfigurasi DB existing di `.env.example`.

## Log dan korelasi

`cmd/server` menulis JSON ke stdout melalui slog; pengumpulan/rotasi dilakukan supervisor operator. `LOG_LEVEL` menerima debug/info/warn/error. Default info: sukses dan redirect INFO; 4xx WARN; 5xx, panic, connection abort dan write failure ERROR. Level warn/error menyaring event lebih rendah tetapi tidak mematikan metrics. Debug saat ini mencakup event yang sama tanpa menambahkan payload.

Satu event `http.request` saat handler selesai:

```json
{"level":"ERROR","msg":"http.request","request_id":"ID_BUATAN_SERVER","route":"/api/pesanan/{id}/pembayaran","method":"POST","status":503,"duration_ms":21.4,"error_class":"dependency_unavailable","response_aborted":false}
```

Field time ditambahkan slog dalam RFC3339 dengan offset. Route selalu template yang terdaftar; semua aset menjadi `static`, API tak dikenal `api.unmatched`. Method tak dikenal menjadi OTHER. Tidak mencatat URL/path resource mentah, query, header, cookie, Origin, user agent, peer IP, actor/customer ID, nama/kontak, body, idempotency key, nominal transaksi, error driver maupun stack/panic value. X-Request-ID dari caller tidak dipercaya; ID respons sama dengan log dan korelasi audit existing.

| Error class | Interpretasi |
|---|---|
| none | Handler selesai tanpa kegagalan yang diamati |
| validation / authentication / authorization | Input ditolak / sesi atau login gagal / izin atau origin/CSRF ditolak |
| not_found / method_not_allowed / conflict / rate_limited | Route/resource hilang, metode salah, version/idempotency conflict, limiter aktif |
| dependency_unavailable / timeout / canceled | Dependency gagal, deadline terlewati atau context request dibatalkan |
| data_integrity / internal | Stored data/constraint tidak sesuai, atau kegagalan internal generik; jangan mengabaikannya |
| panic / connection_aborted / response_write | Panic handler, abort eksplisit atau kegagalan tulis ke klien; hasil transaksi perlu diperiksa |
| shutdown / configuration / database / listener / transport | Klasifikasi lifecycle/transport atau readiness saat draining |

Panic sebelum respons menjadi 500 JSON generik ber-ID; header representasi/session sementara dibersihkan, termasuk Set-Cookie/Location/Content-Encoding. Panic sesudah status/body dikirim tidak dapat diganti menjadi 500: koneksi dibatalkan, status log mempertahankan status yang sudah dikirim dan `response_aborted=true`. Abort sebelum status memakai status=0. Error count tetap bertambah walaupun status semula 2xx. Recovery HTTP tidak memulihkan panic pada goroutine terpisah atau menjamin transaksi belum commit. Jangan mengirim ulang pembayaran dengan key baru hanya karena koneksi terputus.

Event lifecycle: `startup.database_ready`, `startup.failed`, `server.listen_failed`, `server.serve_failed`, `server.stopping`, `server.shutdown_forced`, `database.close_failed`, `server.stopped`. Startup configuration failure hanya mengidentifikasi kelas, tanpa nilai konfigurasi. Driver/listener/transport error mentah tidak dicetak. `http.server_error` adalah diagnostik net/http yang disaring; event sebelum request terbentuk tidak memiliki request ID.

Jika unit systemd memang telah dipasang oleh operator, baca log dengan `journalctl -u NAMA_UNIT_AKTUAL --since '-15 min' -o cat`; nama itu placeholder, bukan unit yang dibuat repository. Di Windows gunakan stdout terminal/proses server yang dijalankan. Cari request ID respons dan cocokkan dengan audit owner di aplikasi. Hindari menyalin log proxy/Cloudflare yang mengandung path token tracking. Proxy harus meredaksi route tersebut dan tidak mencatat cookie/query/body. Retensi log aplikasi tidak diotomatisasi; batasi akses, ukuran dan retensi pada supervisor. Audit persisten memiliki policy terpisah di [audit.md](audit.md).

## Metrics minimal

`GET/HEAD /api/ops/metrics` membutuhkan cookie sesi sah dan **owner aktif saat ini**. Admin/cashier mendapat 403, anonim 401; denial memakai permission `manage_configuration` dan audit existing. Tidak ada endpoint metrics publik atau credential monitoring baru. Owner dapat membuka URL itu pada browser yang sudah login; untuk dua snapshot gunakan GET manual, tanpa menyalin cookie/JWT ke command/history. Snapshot no-store dan query DB untuk authorization tetap diperlukan; saat DB gagal metrics dapat 503 juga. Gunakan log/readiness dan pemeriksaan DB operator pada kondisi itu, jangan membuka endpoint sebagai jalan pintas.

- `started_at`: awal registry proses, sebagai penanda reset.
- `requests`: series route/method/status dengan count, errors, duration_ms_total dan duration_ms_max. Termasuk probe dan request yang ditolak CORS/auth/limit. Error mencakup 4xx/5xx, panic/abort dan write failure. Snapshot tidak mencakup request metrics yang sedang menyusun responsnya sendiri.
- Rata-rata durasi dalam interval = selisih duration_ms_total / selisih count. Max adalah maksimum sejak proses hidup; belum ada histogram/percentile. Baca 5xx/panic terpisah dari 401/403/404 yang dapat normal. Batas registry 512 series termasuk bucket overflow; tidak ada label token/ID/payload.
- `db_pool`: max_open/open/in_use/idle, wait_count/wait_duration_ms dan jumlah koneksi ditutup karena idle/lifetime, dari sql.DB.Stats tanpa query tambahan. In_use mendekati max_open disertai kenaikan wait count/duration menandakan antrean pool; tidak otomatis berarti DB rusak. Nilai null bila adapter test tidak mendukung Stats.
- `business`: orders_created, payments_recorded, refunds_recorded. Bertambah hanya sesudah use case HTTP berhasil dan non-replay; kegagalan/rollback intent/replay tidak menambah. Tanpa nominal, metode, identitas atau referensi order. Tidak mencakup penulisan lewat command lain. Crash di antara commit dan increment dapat kehilangan count.

Semua counter berada di memori satu proses, reset saat restart, tidak diagregasi antar-instance dan **bukan sumber rekonsiliasi finansial**. Gunakan ledger/laporan/audit untuk angka bisnis. Tidak ada Prometheus/exporter, background polling, dashboard baru atau dependency tambahan.

## Database unavailable

1. Bandingkan liveness dan readiness, waktu kejadian, serta error_class dependency_unavailable/timeout pada route bisnis. Jika proses belum start, cari startup.failed/database atau configuration.
2. Dari host aplikasi, periksa koneksi jaringan ke host/port DB yang telah dikonfigurasi, status PostgreSQL, storage/kapasitas koneksi dan sertifikat TLS dengan akses operator. Jangan menaruh password/DSN literal pada argumen/transcript. `verify-full` dan CA/hostname produksi tetap wajib; jangan mengubahnya menjadi disable untuk melewati kegagalan.
3. Jika pool antre, tinjau long-running transaction/lock melalui alat DBA dengan output minimum, serta perubahan trafik/konfigurasi terakhir. Jangan menaikkan pool tanpa memeriksa kapasitas PostgreSQL atau membatalkan transaksi orang lain tanpa peninjauan.
4. Pulihkan dependency/config penyebab. Baca readiness lagi, lalu lakukan smoke terkontrol dengan data test/akun yang diotorisasi. Periksa ledger/audit atas pembayaran berstatus tidak pasti sebelum retry key/body identik. Jangan reset DB, menghapus sesi/audit atau membuat posting pengganti otomatis.

Readiness 200 hanya membuktikan ping. Bila route bisnis masih data_integrity/internal, periksa compatibility schema dan izin role runtime; bukan alasan melakukan migrasi otomatis pada server yang sedang menerima transaksi.

## Migration failure

Server tidak menjalankan migration otomatis. Dari root repository dengan koneksi/config yang memang diotorisasi, inspeksi read-only terlebih dahulu:

```powershell
go run ./cmd/migrate -timeout 30s status
```

Command membaca target dari konfigurasi; bukan otomatis DB test. Jalankan perubahan hanya dalam maintenance yang disepakati, setelah backup dan restore salinan diverifikasi. Gunakan `go run ./cmd/migrate -timeout 2m up` pada target yang sudah ditinjau. Runner mengunci dan menerapkan seluruh pending migration beserta history dalam satu transaksi; saat gagal jangan berasumsi berhasil, periksa status lagi setelah koneksi pulih.

- Lock sibuk: cari migrator lain/maintenance yang aktif; jangan menghapus advisory lock atau menjalankan up berulang serentak.
- Checksum/history berbeda: gunakan executable/revisi SQL yang cocok. Jangan mengedit schema_migrations atau migration yang sudah diterapkan.
- Constraint/SQLSTATE: tinjau penyebab pada salinan privat; perbaikan data/schema membutuhkan rencana yang menjaga ledger/audit dan peninjauan pemilik. Jangan menghapus row untuk memaksa upgrade.
- Koneksi terputus: status dan backup/history menjadi bukti hasil; jangan menebak berdasarkan keluaran terminal terakhir.

Rincian schema 12, cutover legacy dan rollback forward-safe ada di [database.md](database.md). Tidak ada migration baru pada Hari 25; status migrasi operasional tidak diubah/diverifikasi oleh tugas ini.

## High error rate dan panic

Bandingkan dua snapshot proses yang sama atau log dalam rentang waktu untuk route template/status/error class. Abaikan reset counter berdasarkan started_at. Pisahkan lonjakan auth/validation dari 5xx, panic, data_integrity dan write failures. Periksa waktu deployment/config/dependency berubah dan apakah semua route atau hanya satu use case terdampak.

Untuk panic/data_integrity berulang: simpan request ID, waktu, route template dan revisi artifact; hentikan aktivitas yang berisiko menggandakan transaksi dan tinjau ledger/audit dengan owner. Reproduksi dengan data sintetis di local/test; jangan menyalakan body logging atau stack mentah di produksi. Untuk rate_limited, periksa pola trafik dan shared peer reverse proxy sebelum mengubah limiter. Untuk response_write/abort, pastikan hasil bisnis dahulu. Jangan menaikkan timeout, mempercayai X-Forwarded-For atau mematikan CSRF/CSP/auth sebagai perbaikan insiden.

## Shutdown dan rollback

SIGTERM/interrupt memulai draining: readiness menolak, listener berhenti menerima request, request aktif diberi HTTP_SHUTDOWN_TIMEOUT sebelum HTTP Close paksa. Database ditutup tepat sekali setelah fase HTTP; error close tetap dipropagasikan. Pada timeout, koneksi ditutup sehingga request context/query yang kooperatif dibatalkan. Handler yang mengabaikan context dapat masih berjalan setelah force close; runtime DB close juga bergantung pada driver. Log server.shutdown_forced menandai perlunya memeriksa hasil transaksi. Atur grace supervisor lebih panjang daripada shutdown aplikasi dan jangan menggunakan kill paksa sebagai langkah normal.

Rollback operasional, sebelum tindakan nyata:

1. Catat artifact/schema/version worker/config yang sedang berjalan, waktu masalah dan backup terakhir. Hentikan request tulis baru, tunggu penyelesaian request dan rekonsiliasi intent finansial yang tidak pasti. Koordinasikan maintenance dengan pemilik.
2. Pilih artifact sebelumnya yang kompatibel schema 12 dan mempertahankan kontrol cookie/CSRF/security Hari 24. Hari 25 tidak mengubah schema; jangan drop tabel atau menghapus audit. Bila compatibility tidak jelas, pilih forward fix yang ditinjau.
3. Terapkan artifact/config secara atomik sesuai prosedur deployment operator, tanpa menyalin secret ke repository. Worker yang ikut dirilis/dirollback harus memakai versi baru unik; baca [pwa.md](pwa.md). Tidak ada script yang mengeksekusi langkah ini otomatis.
4. Periksa live/ready, log startup, login owner/izin, satu alur test yang diotorisasi dan ledger/audit sebelum membuka kembali operasional. Readiness bukan bukti penerimaan seluruh aplikasi. Pengembalian artifact lama menghilangkan endpoint/log/metrics Hari 25; sesuaikan probe jika rollback memang dipilih.

Restore backup hanya setelah review pemilik, menggunakan database terpisah untuk verifikasi dan rencana rekonsiliasi transaksi setelah backup; restore bukan langkah rutin rollback binary. Bukti deployment nyata, backup/restore drill, supervisor/proxy/Cloudflare dan race detector masih merupakan pekerjaan rilis terpisah.

## Verifikasi lokal terarah

Command resmi dari root repository; tidak membaca `.env` operasional pada unit test:

```powershell
go test ./internal/telemetry ./internal/config ./internal/http/... ./internal/app -skip '^TestPostgres' -count=1 -timeout=60s
go test ./internal/service -run '^TestOperationsAuthorizationUsesCurrentActiveOwner$' -count=1 -timeout=45s
go vet ./internal/telemetry ./internal/config ./internal/http/... ./internal/app ./internal/service ./cmd/server
```

Untuk sandbox Windows, gunakan pengaturan GOCACHE workspace pada README. Test memakai fake/context deadline dan loopback HTTP untuk panic, redaksi, health, izin metrics dan graceful/forced shutdown, tanpa full UI test atau PostgreSQL operasional. Bukti aktual ada di [laporan Hari 25](progress/day-25.md).
