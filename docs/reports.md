# Dashboard dan laporan — Hari 16 dan 22

Backend laporan tersedia untuk owner/admin; UI dashboard, filter, detail, CSV dan cetak tersedia pada Hari 22. Kasir mendapat dashboard shift sendiri tanpa akses laporan global. Schema saat ini **000012**; tugas UI ini tidak mengubah Go, query, endpoint, migration atau dependency. Sumber data tetap tabel domain, bukan data legacy yang belum diimpor. Bukti backend Hari 16 dan batas acceptance UI–database dipisahkan di bawah.

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

## Verifikasi backend — bukti Hari 16

Jalankan dari root repository, dengan toolchain/cache seperti README. Integration wajib `ULAUNDRY_TEST_ADMIN_DSN` ke database postgres pada **cluster test khusus**, bukan `.env` operasional; helper membuat dan membersihkan DB acak.

```powershell
go test ./internal/service -run '^Test(DashboardReportAuthorizationAndFailure|ReportWindowValidation)$' -count=1 -timeout=45s
go test ./internal/http/handler -run '^TestReport' -count=1 -timeout=45s
go test ./internal/service -run '^TestPostgresOperationalReports$' -count=1 -timeout=90s -v
go test ./internal/http -run '^TestPostgresReportHTTP$' -count=1 -timeout=60s -v
```

Fixture PostgreSQL membuktikan awal inklusif/akhir eksklusif hingga mikrodetik, default WITA, DP lintas tanggal, refund/cancel, diskon, ketepatan estimasi, mutasi/biaya, shift semalam, metode tanpa shift, pagination, role, audit rollback, snapshot terhadap writer lain, cancel context dan overflow agregat. HTTPS test memeriksa sesi nyata, JSON/CSV, angka 4000 terbayar/6000 piutang, penolakan anonymous/cashier dan HEAD. Bukti pelaksanaan dan batas penerimaan di [laporan Hari 16](progress/day-16.md).

## UI Hari 22 — tindakan — hasil

| Tindakan | Hasil |
|---|---|
| Dashboard berdasarkan peran | Navigasi Dashboard tersedia untuk owner/admin/cashier. Owner/admin membaca ringkasan global dan dua endpoint detail laporan. Kasir hanya meminta /api/shifts/current: sistem tunai, saldo awal, penerimaan bersih dan mutasi laci shift sendiri, dengan tautan POS/antrean/kas. Tanpa shift, tampil petunjuk buka kas; tidak ada request laporan global atau angka usaha pengganti |
| Ringkasan pemilik/admin | Delapan cards: order dibuat, penerimaan, refund dan net periode; piutang, terlambat dan shift aktif saat pembacaan; selisih penutupan periode. Tabel metode, layanan populer serta rincian biaya/mutasi/penutupan menjaga arti angka pada kontrak backend. Bagian tanpa shift ditandai sudah termasuk total; nilai layanan sebelum diskon bukan omzet pembayaran. Tidak menambahkan grafik/library |
| Date filters | Awal/akhir berupa tanggal bisnis WITA. Tanggal akhir inklusif dikonversi ke batas eksklusif pukul 00.00 hari berikutnya; maksimal 366 hari, termasuk leap year. Zona perangkat tidak menentukan periode. Perubahan tanggal langsung menyembunyikan hasil lama dan menonaktifkan cetak/ekspor sampai Tampilkan laporan berhasil. Tombol Hari ini memakai kalender WITA |
| Detail berhalaman | Dua tabel independen, 20 baris: order created/receivable/overdue dengan filter lima status; ledger posting periode dengan arah, metode, referensi, catatan dan pencatat. has_more dari server menentukan tombol berikutnya. Filter kosong dihilangkan; jenis snapshot tetap seluruh tanggal. Pesanan cancelled tetap memperlihatkan total snapshot tetapi paid/due nol sesuai query, bukan tagihan baru |
| Metadata/konsistensi | Tiap bagian memuat periode, waktu pembacaan WITA, ID dan halaman/jenis filter. Ringkasan/detail/CSV merupakan pembacaan terpisah; refresh/pagination bukan snapshot gabungan yang dibekukan. Tidak menjumlahkan halaman detail menjadi total laporan. Angka uang wajib integer aman JavaScript; nominal nol, net negatif dan selisih negatif tetap ditampilkan, data tidak valid/overflow ditolak sebelum bagian dibuka |
| CSV privat | Tombol CSV halaman ini memakai filter/periode/limit/offset halaman yang dimuat melalui transport cookie existing. UI memeriksa status, Content-Type dan metadata X-Report-* sebelum membuat unduhan Blob; tidak membentuk ulang isi CSV dari HTML. Nama berkas hanya jenis/periode/offset, tanpa PII. encoding/csv dan proteksi apostrof formula tetap dari server. Pesan menyatakan unduhan dimulai, waktu/ID ekspor serta halaman berikutnya bila ada |
| Shift kasir | /shifts.html mempertahankan buka/mutasi/tutup kas sendiri; owner/admin membaca harian seluruh kasir, cashier hanya dirinya. Cards membedakan saldo sesi, penerimaan bersih harian dan mutasi laci. Bagian tanpa shift sudah termasuk total. Metode historis tak dikenal tidak digolongkan sebagai penerimaan teridentifikasi. Baris shift merangkum seluruh sesi yang beririsan tanggal, bukan snapshot akhir hari historis |
| Retry shift | Satu intent key/body/path dibekukan sebelum POST. Double-submit dikunci; network/timeout/408/5xx/JSON invalid menahan form untuk retry eksplisit yang sama. Penolakan setelah hasil tidak pasti tetap menahan key; konflik definitif meminta muat ulang. Hasil idempotent menampilkan ID tanpa posting ulang; keberhasilan transaksi tidak dibatalkan oleh kegagalan refresh. Tidak ada antrean offline atau penyimpanan PII/key pada browser storage |
| Cetak | A4 landscape, header tabel berulang, teks/nilai membungkus, navigasi/form/tombol disembunyikan. Print dashboard mencakup ringkasan dan dua halaman detail yang dimuat; print shift mencakup ringkasan tanggal serta halaman transaksi yang dipilih. Metadata dan catatan cakupan tetap terlihat. Ini bukan cetakan seluruh hasil atau thermal receipt |
| States/aksesibilitas | Loading dan controls lock, empty yang tidak menghapus snapshot piutang, gagal per bagian/retry, offline dan 403/401 ditangani. Tabel lama disembunyikan ketika gagal/ganti filter. Safe DOM memakai createElement/textContent, tabel dapat digeser/fokus keyboard, fokus input dipertahankan sesudah muat ulang. Palet/tokens Hari 18 dipertahankan |

## Verifikasi UI Hari 22 — tindakan — hasil

- Edge headless dengan JSON fixture sintetis — 25 order, penerimaan Rp100.000, refund Rp3.000, net Rp97.000, piutang Rp150.000 dan selisih penutupan −Rp1.000 dirender sesuai respons terkontrol. Net −Rp3.000, nominal nol, cancelled dengan due nol, nilai melewati integer aman dan data kosong diperiksa. Ini bukti tampilan/validasi, bukan rekonsiliasi query/database.
- Role/date/filter — owner/admin dapat membaca laporan; kasir hanya meminta shift sendiri dan submit/ekspor paksa tidak memanggil laporan global. Scope shift sendiri/semua dan penolakan fixture shift asing diperiksa. Kalender perangkat Los Angeles tetap mengirim 26 November WITA sebagai [25 November 16.00Z,26 November 16.00Z). Periode terbalik/lebih dari 366 hari ditolak; leap year, filter jenis/status, pagination 20/5, label metode historis dan tanggal shift lintas tengah malam lulus.
- CSV — browser mengunduh byte identik dari handler Go laporan existing dengan use case sintetis. Nama pelanggan =SUM(1,2), kutipan/koma serta keterangan berawalan whitespace/tab/formula dan newline mendapat escaping/proteksi server; timestamp +08:00, metadata halaman dan tidak adanya token diperiksa. Metadata ekspor tidak cocok ditolak tanpa unduhan. Handler digunakan sebagai generator fixture sementara, tanpa mengubah source backend atau menjalankan suite backend.
- Browser states/transaksi — loading, gagal ringkasan/detail/shift, retry, offline tanpa pengiriman otomatis, forbidden dan expiry lulus tanpa exception JavaScript/console.error. Shift buka/mutasi/tutup, saldo/selisih, double-submit, respons hilang dan replay key/body identik, serta 409 setelah hasil tidak pasti diperiksa.
- Visual/print — viewport 1280/768/360/390/412px tanpa overflow halaman; screenshot dashboard/cards/detail/shift ditinjau. Keyboard Enter/fokus dan region tabel diperiksa. Preview print menyembunyikan controls serta mempertahankan metadata/halaman transaksi. PDF A4 landscape terbuat: fixture dashboard 5 halaman, shift 3 halaman; dimensi/jumlah halaman diperiksa. Printer/Android fisik belum diuji.
- Syntax/dokumentasi — sepuluh HTML dan enam belas JavaScript diperiksa untuk syntax, UTF-8, aset lokal, ID unik, tokens dan scope header; diff/tautan laporan diperiksa. Tidak menjalankan test Go, PostgreSQL, contract suite atau full suite karena query/API tidak berubah. Harness, CSV, screenshot dan PDF sintetis berada di cache luar repository; tidak membaca .env atau DB operasional.

Batas pending shift: key/body hanya bertahan di memori tab; reload/penutupan paksa/expiry dapat menghilangkannya. Periksa transaksi sebelum memasukkan ulang. Validasi form/print/role UI tidak menggantikan izin server; acceptance browser → server → PostgreSQL nyata tetap diperlukan sebelum rilis. Bukti dan sisa pekerjaan ada di [laporan Hari 22](progress/day-22.md).
