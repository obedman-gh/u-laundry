# Kontrak HTTP API — hari 09

Server memakai storage domain dan autentikasi migration 000005. Ini perubahan kontrak dari handler legacy: tidak menerima total/harga/status klien saat create; data/akun pesanan-kasir-keuangan lama belum diimpor. Jangan aktifkan pada data operasional sebelum mapping, backup/restore dan rekonsiliasi; lihat [architecture.md](architecture.md).

Semua body JSON memakai satu Content-Type application/json (charset UTF-8 opsional), maksimal 64 KiB termasuk whitespace, satu objek UTF-8 dan tanpa field asing. Content-Encoding selain identity atau header encoding berulang ditolak. Field duplikat, termasuk escape/alias kapitalisasi dan objek item bersarang, ditolak sebelum use case dipanggil. Rupiah dan gram/jumlah berupa integer. Cookie sesi HttpOnly diperlukan selain login/health/tracking. Bearer/localStorage tidak diterima lagi. JWT memerlukan HS256, issuer/audience/iat/exp/sub/username/role/jti dan registrasi sesi PostgreSQL; role/active tetap dimuat ulang setiap use case. Mutasi privat memerlukan X-CSRF-Token dan Origin persis PUBLIC_BASE_URL; login memerlukan Origin tersebut tanpa sesi awal. Akun baru belum memiliki bootstrap aman (pekerjaan berikutnya); utilitas setup-kasir lama hanya menyentuh tabel legacy.

| Method/path | Scope dan kontrak |
|---|---|
| POST /api/login | `{ "username":"...", "password":"..." }`; cookie sesi + nama/pesan/csrf_token/expires_at, tanpa JWT JSON; failure generik dan 429 rate limit |
| GET /api/session | Sesi aktif; user_id/username/role/csrf_token/expires_at, no-store |
| POST /api/logout | Cookie + X-CSRF-Token/Origin; revoke persisten dan hapus cookie; cookie lama 401 |
| GET /api/health | Ping database; 200 sehat, 503 health_unavailable saat gagal; bukan liveness/readiness terpisah |
| POST /api/customers | Staf aktif; name/contact/notes → customer_id/name |
| POST /api/services | Owner/admin; name/unit/unit_price/turnaround_minutes/active → service_id/version |
| PUT /api/services/{id} | Owner/admin; field konfigurasi + expected `version`; snapshot order lama tetap |
| POST /api/pesanan | CustomerID/item/discount/notes/estimasi → order dan tracking_token sekali |
| GET /api/pesanan | Staf; limit/offset/status/customer_id, array ringkasan kompatibel field daftar lama |
| GET /api/pesanan/status?q=processing | Alias deprecated; q wajib status domain, pagination sama; penerus GET /api/pesanan?status=processing |
| GET /api/pesanan/{id} | Staf; satu objek detail termasuk version; **tidak publik lagi** |
| GET /api/pesanan/{id}/struk | Staf; HTML escaped untuk print; **tidak publik lagi** |
| PUT /api/pesanan/{id} | Staf; status dan expected version; bukan mencatat pembayaran |
| POST /api/pesanan/{id}/cancel | Owner/admin; version/reason; belum ada pembayaran |
| POST /api/pesanan/{id}/pembayaran | Staf; amount/method + header Idempotency-Key wajib |
| GET /api/tracking/{token} | Publik capability; hanya status/estimated_ready_at/ready_at/collected_at |
| GET /api/laporan/neraca | Owner/admin; from/to RFC3339 opsional berpasangan, default hari berjalan WITA; cash inflow/outflow/balance + report_id |

Contoh create order (ID layanan/customer dari respons create/configure):

```json
{
  "customer_id": "CUSTOMER_ID",
  "items": [{"service_id":"SERVICE_ID", "grams":2500}],
  "discount": 0,
  "notes": "Pisahkan warna",
  "estimated_ready_at": "2026-11-09T10:00:00+08:00"
}
```

Layanan kg memakai grams; piece memakai pieces, field lain nol. Estimasi boleh tidak dikirim dan dihitung dari durasi katalog. ID order, nama customer, harga, total dan status bukan field bebas dari request; customer_id menunjuk customer yang sudah tersimpan. Diskon nominal memerlukan owner/admin dan discount_reason. Total response dihitung server. Contoh update progres: `{"status":"processing","version":1}`. Lima status: received → processing → ready → collected, received/processing dapat cancelled melalui endpoint cancel. Pengambilan harus lunas. Estimasi dan batas periode laporan dinormalisasi UTC/mikrodetik; periode yang menjadi kosong setelah normalisasi ditolak.

Pembayaran: `{"amount":10000,"method":"cash"}` dengan Idempotency-Key yang dibuat klien dan dipakai kembali hanya untuk payload bisnis sama. Metode manual cash/bank_transfer/qris, tanpa gateway. Replay mengembalikan payment_id/amount yang sama dan replayed=true; tidak menambah ledger/audit. Status `Sudah Lunas` lama tidak diterima sebagai status operasional. Create order belum idempotent; jangan retry buta setelah commit dengan hasil jaringan tidak diketahui.

Pagination limit default 20/maksimum 100, offset minimal 0. Detail sekarang satu objek, bukan array satu elemen. Ringkasan tetap id_pesanan/nama_customer/layanan/total_biaya/status untuk halaman daftar yang ada, ditambah payment_status/version. Nilai status sekarang kode domain; update frontend lengkap/diskon/POS belum dibuat. Konsumen JavaScript harus memperhatikan batas integer aman saat menangani nominal BIGINT besar; seluruh kalkulasi server tetap int64.

## Error dan correlation ID

Semua kegagalan endpoint API memakai bentuk berikut, termasuk JWT/CORS, 404, method salah, dependency failure dan panic sebelum respons dimulai:

```json
{"error":{"code":"validation_failed","message":"Data tidak memenuhi ketentuan.","request_id":"ID_ACAK_DARI_SERVER"}}
```

`X-Request-ID` selalu diterbitkan server pada router, sama dengan `error.request_id`; nilai header dari klien tidak dipantulkan. ID untuk correlation respons ini belum menjadi request logging terstruktur. Klien membaca `error.message` sebagai teks, bukan HTML; halaman login sudah disesuaikan. Bentuk lama error string/teks tidak lagi menjadi kontrak API. Respons sukses tetap DTO endpoint, tanpa envelope baru.

| HTTP | code | Arti |
|---|---|---|
| 400 | invalid_json / unknown_field / invalid_body | JSON rusak/ambigu, field asing, body tidak terbaca |
| 400 | validation_failed / invalid_path_parameter | Nilai/parameter/batas pagination tidak sah |
| 401 | unauthorized | Login/sesi tidak sah; detail akun tidak diungkap |
| 403 | forbidden / origin_forbidden / preflight_forbidden / csrf_failed | Role atau origin/preflight ditolak |
| 404 | not_found | Resource/token/endpoint tidak ditemukan |
| 405 | method_not_allowed | Path API dikenal, method salah; Allow berisi method sah |
| 408 | request_canceled | Context permintaan dibatalkan |
| 409 | conflict | Version stale, transisi atau payload idempotency bertentangan |
| 429 | rate_limited | Percobaan login dibatasi; Retry-After tersedia |
| 413 | payload_too_large | Body melewati 65.536 byte, termasuk stream tanpa Content-Length |
| 415 | unsupported_media_type / unsupported_content_encoding | Tipe/charset/encoding tidak didukung |
| 500 | internal_error | Error internal, storage constraint/invalid stored data, panic atau encoding response gagal |
| 503 | service_unavailable / health_unavailable | Repository atau health dependency tidak tersedia |
| 504 | deadline_exceeded | Context deadline habis |

Message domain/repository/driver, stack trace, nilai panic, password/hash dan input ditolak tidak disalin ke error. Error tidak menjamin operasi belum commit bila respons jaringan gagal; khususnya create order/customer belum idempotent. Panic setelah header dikirim membatalkan koneksi memakai ErrAbortHandler, tidak menulis JSON kedua atau mengubah sukses yang sudah dikirim. Error parsing pada protokol HTTP oleh net/http/proxy dan redirect normalisasi URL bukan respons endpoint JSON.

## Request, route dan kompatibilitas

- Query daftar hanya limit/offset/status/customer_id; alias lama memakai q menggantikan status. Query unknown, duplikat, kosong, invalid escape/format ditolak. Limit 1–100 (default 20); offset integer nonnegatif yang dapat direpresentasikan int platform. Overflow ditolak, bukan di-clamp. Laporan hanya from/to RFC3339 berpasangan dan berurutan setelah normalisasi UTC/mikrodetik.
- ID path dan Idempotency-Key: 1–100 karakter ASCII huruf/angka/titik/underscore/hyphen. Payment memerlukan tepat satu header Idempotency-Key. Create payment 201; replay yang diverifikasi use case 200 dengan replayed=true. Ini bukti kontrak HTTP, bukan pengujian ulang atomicity/concurrency storage.
- Tracking token harus Base64 URL tanpa padding canonical dari 32 byte (43 karakter); token salah berbentuk 404 yang sama. Token tidak dipantulkan. Referrer-Policy no-referrer pada respons tracking.
- GET tetap mendukung HEAD dengan status/header setara tanpa body; auth tetap berlaku. Method salah pada path dikenal memberi 405 dengan Allow. URL /api dan /api/* tidak pernah dilayani FileServer, termasuk jika file dengan nama itu ada.
- Alias GET /api/pesanan/status dipertahankan dengan header Deprecation bertanggal 7 November 2026 UTC dan Link rel=successor-version ke /api/pesanan. Tidak ada janji tanggal penghapusan.
- Respons JSON dan struk memakai Cache-Control no-store dan X-Content-Type-Options nosniff. JSON di-encode sebelum header sukses dikirim; HEAD mempunyai Content-Length representasi tanpa body. Aset statis mempertahankan handler dan tema lama.
- CORS memakai allowlist origin eksak dan Vary. Preflight GET/HEAD/POST/PUT untuk Content-Type/Authorization/Idempotency-Key/X-CSRF-Token mendapat 204 sebelum auth/router; preflight terlarang 403. Origin/Authorization berulang ditolak. Trusted origin dapat membaca X-Request-ID, Deprecation, Link dan Location. Sukses preflight tidak membuktikan path/method tersedia atau izin bisnis diberikan.
- UI login/daftar memakai cookie same-origin, menghapus token localStorage lama, memuat CSRF lewat /api/session dan menyediakan logout. Daftar dirender dengan DOM/textContent, tanpa HTML dari data. UI POS/UX lengkap dan peninjauan XSS/CSP tetap tahap berikutnya. Lihat [security.md](security.md) untuk migrasi sesi, konfigurasi, audit dan batas limiter.

## Verifikasi terarah

Jalankan dari root repository:

```powershell
go test ./internal/http/... ./internal/app -count=1 -timeout=45s
node --test scripts/frontend-contract.test.cjs
go vet ./internal/http/... ./internal/app
go build ./...
```

Unit test memakai fake use case/lifecycle dan DOM sintetis. Integration autentikasi opt-in memakai PostgreSQL test disposable dan HTTPS lokal; command/bukti ada di [security.md](security.md) dan [laporan hari 09](progress/day-09.md). Browser nyata, printer, race dan release checks tidak dijalankan.

