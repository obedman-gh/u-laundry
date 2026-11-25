# Dashboard dan laporan — Hari 16

Backend laporan tersedia untuk owner/admin. Halaman dashboard, kontrol export dan print laporan dijadwalkan Hari 22; AC11 belum ditutup seluruhnya. Schema tetap **000011**, tanpa migration atau dependency baru. Sumber data hanya tabel domain, bukan pesanan/keuangan legacy yang belum diimpor.

## Waktu dan arti angka

Semua endpoint baru menerima `from`/`to` RFC3339 berpasangan dengan offset zona waktu. Tanpa keduanya, periode adalah hari berjalan **Asia/Makassar (WITA, UTC+08:00)**. Interval `[from,to)` mencakup awal dan mengecualikan akhir, maksimal 366 hari; respons mengembalikan batas UTC, `as_of`, `timezone` dan ID laporan. Tidak ada parameter untuk mengubah zona bisnis atau waktu server dari klien.

| Angka dashboard | Arti dan sumber |
|---|---|
| `orders.created` | Jumlah order dibuat dalam periode, termasuk yang kemudian dibatalkan |
| `orders.received/processing/ready` | Antrean seluruh order pada snapshot saat laporan dibaca; tidak dibatasi tanggal pembuatan |
| `orders.receivable_orders/receivable` | Order non-cancelled yang masih berutang dan jumlah `total - pembayaran belum direfund`, seluruh tanggal pada snapshot; DP dan diskon tersimpan diperhitungkan |
| `orders.overdue` | Status received/processing dengan estimasi **lebih awal** dari `as_of`; tepat pada estimasi belum terlambat. Ready/collected/cancelled dikecualikan |
| `money.payments/refunds/net` | Penerimaan/refund berdasarkan timestamp ledger dalam periode; net = payments − refunds, dapat negatif. Receipt yang kemudian direfund tetap termasuk penerimaan pada tanggal asal |
| `money.expenses` | KREDIT selain refund dan mutasi shift; bukan pengurang penerimaan bruto |
| `money.cash_in/cash_out` | Mutasi laci berdasarkan relasi shift_cash_movements; dipisahkan dari omzet dan biaya |
| `money.unassigned_payments/unassigned_refunds` | Bagian penerimaan/refund tanpa shift_id; termasuk dalam payments/refunds, jangan dijumlahkan kembali |
| `methods` | Empat bucket cash/bank_transfer/qris/unknown dari receipt ledger; masing-masing payments dan refunds, nol bila kosong. Mutasi/biaya bukan receipt dan tidak masuk bucket ini |
| `shifts.opened/closed` | Jumlah pembukaan/penutupan yang timestamp-nya berada dalam periode, termasuk shift semalam yang baru ditutup |
| `shifts.active` | Jumlah shift yang masih terbuka pada snapshot, seluruh tanggal |
| `shifts.system/physical/difference` | Jumlah snapshot penutupan **yang ditutup dalam periode**, bukan saldo laci aktif. Saldo awal/penutupan tidak menjadi omzet |
| `popular_services` | Sepuluh kelompok snapshot service_id/nama/unit dengan order terbanyak yang dibuat dalam periode, selain cancelled; urutan seri ID/nama/unit. Nama/unit lama tetap dan dapat membentuk kelompok berbeda sesudah revisi katalog |

Layanan populer menghitung `orders` berbeda per kelompok, `pieces`/`grams` integer dan `gross_before_discount` dari total baris sebelum diskon order. Angka ini bukan omzet pembayaran. Satuan API `kg`/`item`; penyimpanan `piece` dipetakan ke `item` seperti katalog. Nilai uang adalah integer rupiah. DEBIT historis tanpa receipt atau mutasi teridentifikasi tidak dianggap omzet; tetap terlihat pada detail ledger dan buku kas `/neraca`.

Antrean, piutang, keterlambatan dan shift aktif adalah **keadaan saat pembacaan**, bukan rekonstruksi keadaan pada akhir periode historis. `as_of` menandai waktu pembacaan dan pembanding keterlambatan. Pembayaran baru di luar periode dapat mengurangi piutang sekarang tanpa mengubah omzet periode tersebut. Data demo/test sintetis tidak menggantikan rekonsiliasi data usaha nyata.

## Endpoint dan detail

```text
GET /api/laporan/dashboard
GET /api/laporan/dashboard?from=2026-11-24T00:00:00%2B08:00&to=2026-11-25T00:00:00%2B08:00
GET /api/laporan/orders?kind=created&limit=20&offset=0
GET /api/laporan/orders?kind=receivable&status=processing&limit=20&offset=0
GET /api/laporan/orders?kind=overdue&limit=20&offset=0
GET /api/laporan/ledger?limit=20&offset=0
GET /api/laporan/ledger?limit=100&offset=0&format=csv
```

Encode `+` zona waktu menjadi `%2B` dalam query URL. Semua endpoint memerlukan cookie sesi aktif; owner/admin diperiksa kembali pada service. Cashier mendapat 403 dan tanpa sesi mendapat 401, termasuk CSV/HEAD. Query asing/duplikat, status/jenis/format asing, tanggal tanpa zona, pasangan tanggal tidak lengkap, periode terbalik/lebih panjang dari batas dan pagination invalid ditolak. Tidak ada actor yang dipilih melalui query.

Detail order: `kind=created` default memilih tanggal pembuatan dalam periode; `receivable`/`overdue` memilih kondisi snapshot seluruh tanggal. Filter `status` opsional adalah enum domain; `from/to` pada dua jenis terakhir hanya metadata periode, bukan filter tanggal order. Sort order `created_at DESC,id DESC`. Kolom: ID, nama pelanggan, status, total, paid, due, created_at dan estimated_ready_at; tanpa kontak, alamat, catatan atau tracking token.

Detail ledger: timestamp posting dalam periode, urutan `occurred_at,id` menaik. ID, relasi order/payment/refund/shift, arah, metode, nominal, keterangan, pencatat dan timestamp; tanpa idempotency key. Nominal selalu positif; arah DEBIT/KREDIT menjelaskan arus. Keterangan dapat berisi alasan operasional, sehingga tetap privat.

Detail JSON berbentuk `id/from/to/as_of/timezone/kind/limit/offset/has_more/items`. Limit default 20, rentang 1–100; offset minimal nol. Repository mengambil satu baris tambahan untuk has_more. Kosong menghasilkan `items: []`; metode dashboard tetap empat bucket nol, layanan populer `[]`.

## CSV untuk rekonsiliasi

`format=json` default; `format=csv` tersedia pada kedua endpoint detail, **hanya halaman yang diminta**, bukan seluruh database. Nama unduhan mencantumkan offset. Kolom Indonesia, nominal integer tanpa pemisah ribuan, timestamp RFC3339 WITA. CSV tidak mengekspor token/kontak pelanggan atau secret; nama pelanggan hanya pada laporan order.

Respons `text/csv; charset=utf-8`, Content-Disposition attachment, `no-store`, nosniff dan request ID. Header `X-Report-ID/From/To/As-Of/Limit/Offset/Has-More` mendeskripsikan halaman. Saat Has-More true, minta offset berikutnya dengan filter yang sama. Header ini ditujukan klien same-origin; allowlist expose CORS existing tidak diperluas. HEAD memberi header tanpa body, dengan pemeriksaan izin dan audit yang sama.

Encoding memakai standard library `encoding/csv` dengan CRLF dan escaping delimiter/kutipan. Semua teks yang awal efektifnya `=`, `+`, `-`, `@` sesudah whitespace/control, atau mengandung tab/CR/LF, diberi prefiks apostrof untuk mencegah formula injection. Nominal ditulis dari integer bertipe, bukan input teks. Prefiks tersebut sengaja menjadi bagian nilai ekspor.

Setiap halaman mempunyai snapshot/ID sendiri. Pagination berulang bukan snapshot ekspor massal yang dibekukan; transaksi baru dapat mengubah posisi order/angka antarhalaman. Untuk rekonsiliasi menyeluruh gunakan periode selesai saat tidak ada penulisan dan periksa kelengkapan halaman. CSV bukan backup database atau laporan akuntansi double-entry/akrual/settlement bank.

## Konsistensi, kegagalan dan query

Use case memakai `UnitOfWork.View` (PostgreSQL read-only REPEATABLE READ): auth dan kelima query dashboard berbagi snapshot. Setelah read transaction berhasil commit, transaksi audit terpisah memeriksa ulang staf dan mencatat report_generated. Data baru diberikan setelah audit commit; query/scan/rows/close/read commit/audit commit gagal menghasilkan error, bukan angka nol atau hasil parsial. ID laporan dapat ditelusuri ke audit tanpa menyimpan data laporan/PII dalam metadata. Laporan `/neraca` existing tetap mempertahankan kontraknya.

Query agregat tetap berjumlah lima, tanpa query per order/shift. Nilai eksternal selalu bind parameter. SUM PostgreSQL dikonversi ke BIGINT dan scan int64; overflow menjadi kegagalan. Indeks existing order created/status/customer, pembayaran per order, order_items PK/service, ledger occurred_at/order/shift dan cash_shifts opened_at tetap dipakai sesuai filter. Agregat keadaan saat ini tetap membaca populasi order/shift. Fixture kecil dan kebutuhan bisnis saat ini tidak membenarkan indeks/migration/benchmark baru; EXPLAIN pada dataset operasional anonim menjadi pekerjaan bila latensi atau pertumbuhan data menunjukkan masalah.

## Verifikasi terarah

Jalankan dari root repository, dengan toolchain/cache seperti README. Integration wajib `ULAUNDRY_TEST_ADMIN_DSN` ke database postgres pada **cluster test khusus**, bukan `.env` operasional; helper membuat dan membersihkan DB acak.

```powershell
go test ./internal/service -run '^Test(DashboardReportAuthorizationAndFailure|ReportWindowValidation)$' -count=1 -timeout=45s
go test ./internal/http/handler -run '^TestReport' -count=1 -timeout=45s
go test ./internal/service -run '^TestPostgresOperationalReports$' -count=1 -timeout=90s -v
go test ./internal/http -run '^TestPostgresReportHTTP$' -count=1 -timeout=60s -v
```

Fixture PostgreSQL membuktikan awal inklusif/akhir eksklusif hingga mikrodetik, default WITA, DP lintas tanggal, refund/cancel, diskon, ketepatan estimasi, mutasi/biaya, shift semalam, metode tanpa shift, pagination, role, audit rollback, snapshot terhadap writer lain, cancel context dan overflow agregat. HTTPS test memeriksa sesi nyata, JSON/CSV, angka 4000 terbayar/6000 piutang, penolakan anonymous/cashier dan HEAD. Bukti pelaksanaan dan batas penerimaan di [laporan Hari 16](progress/day-16.md).
