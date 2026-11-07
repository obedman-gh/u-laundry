# Arsitektur ULaundry — hari 09

Go/net/http, PostgreSQL dan frontend sederhana dipertahankan. Service/use case kini aktif pada composition root dan handler. Tidak ada SQL, handle DB, atau pengaturan transaksi dalam handler. Ini belum merupakan bukti kesiapan produksi atau migrasi data operasional.

## Arah dependency yang sudah tersedia

```text
cmd/server
  -> config + platform.OpenDatabase
  -> postgres.New(pool) + clock/ID/token/password/JWT adapters
  -> service.Customers / Catalog / Orders / Payments / Tracking / Reports / Auth / Health
  -> app.New(settings, pool, handler.Dependencies)

handler -> use-case interfaces -> service -> repository contracts -> domain
postgres -> repository + domain + database/sql + lib/pq -> PostgreSQL
middleware -> JWT validation + actor ID in context
```

`cmd/server` menyusun dependency konkret. App hanya memiliki lifecycle server/pool dan router; pool ditutup setelah HTTP drain. Handler menerima interface use case untuk decode/response/template. Service tidak mengimpor HTTP, database/sql atau adapter postgres. Repository contracts mengimpor domain, bukan adapter. Domain hanya standard library. Tidak ada DB global, framework DI atau dependency baru.

## Pemilik workflow

| Service | Intent dan hasil |
|---|---|
| Customers | Memuat staf aktif, membuat/menormalisasi customer, menyimpan dan mengaudit dalam satu transaksi |
| Catalog | Owner/admin mengonfigurasi layanan baru atau memperbarui harga/satuan/durasi/aktif dengan lock + expected version; menyimpan audit |
| Orders | Mengambil customer/katalog/actor tepercaya, menghitung total/snapshot domain, membuat hash tracking dan audit; change status/cancel memakai lock/version dan aturan domain |
| Payments | Memuat actor/order, mengunci sebelum memeriksa replay, memvalidasi pembayaran terhadap domain, lalu menyimpan payment + ledger + audit dalam satu transaksi |
| Tracking | Hash token valid → projection status/estimasi/milestone, tanpa ID pesanan, customer, kontak, catatan, layanan, harga atau payment |
| Reports | Owner/admin mendapatkan cash-flow ledger periode [from,to) dan audit akses laporan; bukan menghitung pendapatan dari label status order |
| Auth | Kredensial staf domain aktif, verifikasi bcrypt, penerbitan JWT dan audit login sukses; kegagalan username/password generik |
| Health | PingContext melalui port Pinger; handler hanya membentuk respons health |

Tidak ada service raksasa yang memuat seluruh operasi. Runtime bersama hanya clock dan generator; policy authorization/audit dipakai ulang. Get/list/receipt melewati Orders.Get/List dengan pemeriksaan staf aktif. Kontrak HTTP rinci ada di [api.md](api.md).

## Authorization intent

Use case menerima actor ID, bukan role pilihan klien. Pada setiap command, actor dimuat melalui repository di dalam transaction/view dan diperiksa active + perannya. JWT membawa subject ID; middleware hanya menerima HS256 dengan expiration dan subject nonkosong. Claim role tidak dipercaya. Body/header selain bearer tidak dapat memilih identitas aktor.

| Intent | Cashier | Admin/Owner |
|---|---|---|
| Create customer/order, perubahan progres, record payment, baca daftar/detail/struk | Ya | Ya |
| Diskon nominal | Tidak | Ya, alasan wajib |
| Configure service, cancel order, generate financial report | Tidak | Ya |
| Tracking publik | Token capability saja; tidak memberi akses internal | Token capability saja |

Pembatalan tetap hanya pada transisi domain yang sah dan tanpa pembayaran; role owner bukan bypass invarian. Mengambil order harus lunas. `cancelled` tidak diterima melalui ChangeStatus; Cancel memerlukan alasan. Matriks ini kebijakan awal eksplisit, belum user-management/permission system lengkap. Audit actions mencakup customer_created, service_configured, order_created/status_changed/cancelled, discount_approved, payment_recorded, report_generated dan staff_logged_in.

## Transaction boundary dan hasil

Setiap use case mutasi memakai UnitOfWork.WithinTransaction. Semua repository pada callback berbagi sql.Tx READ COMMITTED; order/configuration dikunci FOR UPDATE dan perubahan memakai version. Callback mengembalikan setiap error. Tidak ada hasil sukses, ID baru, JWT atau tracking token yang dikembalikan bila callback/commit gagal. Efek generator lokal boleh mengonsumsi ID pada percobaan yang dibatalkan; tidak ada network side effect di callback.

View memakai read-only REPEATABLE READ untuk state konsisten header/item/payment. GetOrder membangun OrderState dan RestoreOrder memvalidasi snapshot/histori tanpa membaca harga atau nama customer terkini. Service/report tidak menggunakan clock global: clock, ID dan token generator diinjeksi; bootstrap menyediakan system clock dan crypto/rand (ID 16 byte, tracking 32 byte). Waktu server dinormalisasi UTC/mikrodetik sesuai presisi PostgreSQL.

Payments mengunci order sebelum lookup key agar dua request identik tidak gagal karena saldo yang telah berubah. Replay order/key/amount/method/actor sama mengembalikan payment asli tanpa ID/audit/ledger baru. Payload bisnis berbeda menjadi conflict. Payment/ledger serta audit berada pada callback yang sama; adapter juga mengulangi pemeriksaan domain/lock sebagai pertahanan storage. UNIQUE(order_id,idempotency_key), UNIQUE ledger.payment_id dan FK komposit payment/order/amount melindungi posting. Tidak ada klaim jaminan bagi penulis SQL manual yang melanggar boundary ini.

Jika commit putus dan hasilnya tidak diketahui, periksa key pada koneksi baru sebelum retry. Tidak ada automatic retry callback/side effect. Create customer/order belum memiliki idempotency key; kegagalan jaringan setelah commit harus ditinjau sebelum mengirim create ulang. Refund/reversal, shift dan koreksi keuangan belum tersedia.

## Query, error dan batas presentation

Semua SQL hanya di adapter PostgreSQL, memakai context dan bind. Scan/rows.Err/rows.Close/RowsAffected, commit dan rollback-relevant errors diperiksa. Error repository hanya Kind/Operation/Retryable dengan safe sentinels; driver tidak di-Unwrap ke handler. Domain errors bertipe; boundary HTTP memiliki mapping dasar validation 400, forbidden 403, not-found 404, conflict 409, deadline 504, selain itu 500 generik. Hari 08 menuntaskan DTO boundary dan error `error.code/message/request_id` dengan X-Request-ID server pada seluruh error endpoint, termasuk auth/CORS/router. Repository unavailable dipetakan 503, canceled 408 dan invalid stored data tetap 500; rincian kontrak di api.md.

Daftar memakai status/customer WHERE parameter, LIMIT default 20/maksimal 100, OFFSET nonnegatif dan created_at DESC,id DESC; nama layanan berasal dari snapshot item. Slice kosong menjadi []. Pagination offset antar-request masih dapat bergeser jika ada data baru. Layanan aktif/search customer/audit/ledger tetap kontrak repository; tidak ada generic CRUD/delete API.

Handler membatasi JSON UTF-8 64 KiB, membutuhkan application/json, menolak unknown field/multiple JSON/duplicate keys (termasuk nested dan alias kapitalisasi). Router memisahkan namespace API dari file statis, memberi JSON 404/405 dengan Allow dan HEAD tanpa body. RequestContext terluar menerbitkan correlation ID dan mengubah panic sebelum header menjadi safe 500; panic setelah respons dimulai mengakhiri koneksi. DTO create tidak memiliki harga/total/status atau actor dari klien. Struk memakai html/template escaping dan auth. Tracking hanya projection minimal, raw token diberikan sekali saat create dan tidak disimpan; database menyimpan SHA-256. Respons bisnis no-store. Token tracking belum punya expiry/rotation; reverse proxy/access log tidak boleh mencatat raw token URL.

Report default adalah hari berjalan WITA; rentang eksplisit RFC3339 diperbolehkan dengan batas akhir eksklusif. Inflow/outflow int64 nonnegatif; saldo dapat negatif. Satu query agregat memakai snapshot statement yang sama, kemudian audit report dalam transaksi. Overflow/kegagalan scan tidak diubah menjadi nol. Ini buku kas sederhana, bukan double-entry atau laporan revenue akrual.

## Storage dan kompatibilitas deployment

Migration 000004 menambah version layanan, order_tracking_tokens dan perluasan allowed audit actions. Constraint audit diganti dengan superset yang tetap menerima event lama; tabel/row/ID/sequence legacy tidak dikonversi atau dihapus. Migration 000001–000003 tetap immutable. Audit UPDATE/DELETE diblok trigger; hak TRUNCATE/DDL tetap harus dibatasi saat deployment.

**Server hasil hari 07 membaca/menulis tabel domain** customers/services/staff/orders/order_items/payments/ledger_entries/audit_events, bukan pesanan/kasir/keuangan. Tidak ada fallback akun legacy, import otomatis, dual-write atau penggabungan dua ledger. Akun kasir lama dan token tanpa subject tidak otomatis berlaku. Menyalin file kode ini ke server dengan data lama bukan proses import yang sah.

Sebelum aktivasi operasional: hentikan penulisan, backup dan restore salinan, inventaris/mapping akun/status/customer/layanan/payment legacy, rekonsiliasi uang, import melalui migration/command yang ditinjau, siapkan akun staf domain, verifikasi role dan pilih satu writer. Prosedur import/secure bootstrap belum dibangun; jangan menyamakan tabel domain kosong dengan hilangnya data legacy. Data asli tetap tersedia. Rollback binary lama tidak membaca transaksi baru di tabel domain; rekonsiliasi/recovery perlu direncanakan sebelum cutover. Lihat [database.md](database.md).

## Verifikasi dan risiko tersisa

Unit fake repository memeriksa calculation, authorization, dependencies/failure propagation, rollback intent, stale/forbidden transition, cancellation, payment replay, report dan privacy projection. Fake membuktikan intent, bukan rollback PostgreSQL. Satu alur integration terisolasi membuktikan payment-ledger-audit rollback saat audit gagal, retry/replay, report/tracking, serta upgrade 3→4 yang mempertahankan history/data legacy. Tidak menjalankan seluruh integration/UI/acceptance.

Permission/user lifecycle/bootstrap, public tracking rotation, frontend POS lengkap, printer, production TLS/roles/restore/import/cutover dan release checks masih terbuka. Login rate limit, cookie/CSRF/session dan safe DOM daftar sudah diperbarui pada hari 09; tidak ada klaim produksi aman. Command/bukti aktual ada di README dan [laporan hari 09](progress/day-09.md).

## Autentikasi/session hari 09

internal/security memuat policy JWT/cookie/CSRF, limiter dan port Store khusus sesi/event autentikasi. Adapter PostgreSQL Store menerapkan registrasi sesi+audit dan pencabutan+audit atomik; handler tetap tidak memiliki SQL. Auth use case existing memverifikasi credential dan audit bisnis; fingerprint credential dibawa internal ke registrasi agar perubahan password di antara fase tidak membuka sesi dari credential lama. Composition root menginjeksi satu manager ke handler/middleware; tidak ada jalur Bearer tanpa registrasi.

Migration 000005 menambah staff_sessions/auth_events dan trigger pencabutan ketika credential/role/status/username berubah. JWT hanya di cookie HttpOnly; role/active/fingerprint diperiksa pada setiap request dan permission tetap di service. Frontend tidak menyimpan JWT; GET /api/session memberi identitas/CSRF, POST /api/logout mencabut sesi persisten. Rate limit per proses dan risiko multi-instance/proxy, kebijakan password baru, audit anonim dan batas cutover dijelaskan di [security.md](security.md). Histori batas transaksi bisnis hari 07/08 tetap berlaku; policy autentikasi terbaru mengikuti bagian ini.
