# Aturan domain laundry — Hari 05

`internal/domain` adalah aturan bisnis Go murni, memakai standard library. Repository hari 06 dan service/HTTP hari 07 sudah tersambung; lihat [architecture.md](architecture.md). Server domain menghitung total katalog, memisahkan payment/status dan memeriksa actor/version/audit pada use case. DTO `models/` dan data legacy tetap di repository sebagai artefak transisi, tetapi tidak dibaca handler/server baru. Import data dan bootstrap akun belum dilakukan; lihat [api.md](api.md) sebelum memakai client lama.

Aturan berikut sudah dipakai kode server baru, tetapi belum diterapkan ke data operasional: gram sebagai presisi berat, pembulatan per baris, diskon nominal owner/admin, DP diperbolehkan, pengambilan harus lunas, dan pembatalan hanya sebelum siap serta tanpa pembayaran. Cancel/configuration/report hanya owner/admin; cashier dapat operasi umum. Tinjau kebijakan ini sebelum cutover nyata. Minimum kg/tarif, diskon persentase/promo, kredit pelanggan dan refund belum disepakati/dibangun.

## Model dan tanggung jawab

| Model | Isi dan aturan utama |
|---|---|
| `Customer` | ID, nama, kontak opsional, catatan, created/updated time; kontak kosong untuk pelanggan datang langsung |
| `Service` | ID/nama, satuan kg atau piece, harga per satuan, durasi pengerjaan positif, aktif, timestamps |
| `Order` | Snapshot pelanggan/item/harga/diskon, total terhitung, status operasional, estimasi/milestone, penerima dan pembayaran tercatat; field internal dilindungi |
| `OrderItem` | Snapshot ID/nama/satuan/harga layanan, gram atau jumlah satuan, total baris terhitung; perubahan katalog tidak mengubah order lama |
| `Payment` | Penerimaan pembayaran positif: order ID, metode manual, pencatat/waktu, ID dan idempotency key; bukan transaksi gateway pending |
| `LedgerEntry` | Buku kas: nominal positif, DEBIT masuk/KREDIT keluar, deskripsi, pencatat/waktu, order/payment reference opsional; bukan akuntansi double-entry |
| `Staff` (user staf) | ID/username/nama, peran owner/admin/cashier, aktif dan timestamps. Password/hash/JWT/session berada di lapisan autentikasi, tidak masuk model ini |
| `AuditEvent` | ID actor/objek, jenis objek dan aksi terkontrol, waktu; tanpa metadata bebas, credential, nama/kontak pelanggan |

Struct referensi menyediakan `Validate`; constructor order/item dan setiap operasi pembayaran/status mengulangi pemeriksaan kritis. `Order` dibuat melalui `NewOrder`; getter item/payment mengembalikan salinan, dan operasi gagal tidak mengubah state. `RestoreOrder` hari 06 memvalidasi snapshot/histori tanpa memakai harga/role terkini. Uniqueness antar-record dilindungi constraint storage baru; autentikasi actor dan otorisasi route tetap tanggung jawab service.

## Status operasional dan transisi

| Kode | Tampilan Indonesia | Tujuan yang diperbolehkan |
|---|---|---|
| `received` | Diterima | `processing`, `cancelled` |
| `processing` | Diproses | `ready`, `cancelled` |
| `ready` | Siap diambil | `collected` |
| `collected` | Selesai/diambil | Terminal |
| `cancelled` | Dibatalkan | Terminal |

Status awal selalu `received`, ditentukan domain. Transisi mundur, lompatan, status asing dan transisi ke status yang sama ditolak. `CheckTransition` memeriksa tabel; `Order.Transition` juga memeriksa syarat instance:

- Waktu perubahan wajib dan tidak mendahului pembaruan terakhir; waktu sama diperbolehkan. Clock/ID berasal server/use case, tidak dibuat melalui `time.Now()` di domain.
- Menjadi ready mencatat `ReadyAt`, diambil mencatat `CollectedAt`, pembatalan mencatat `CancelledAt`/alasan. Waktu tidak dihitung ulang pada kegagalan/retry.
- Pengambilan memerlukan saldo tagihan nol. Pembatalan memerlukan alasan 1–250 karakter dan belum ada pembayaran; siap diambil tidak dapat dibatalkan pada fondasi ini.
- Perubahan status tidak mengubah nominal, mencatat payment, atau membuat ledger. Pembatalan menyimpan snapshot penawaran semula, tanpa memalsukan pelunasan; laporan piutang mendatang harus mengecualikan order cancelled.

`PaymentStatus` terpisah dan diturunkan dari total dan payment tercatat: `unpaid`, `partially_paid`, `paid`. Total nol (tarif gratis atau diskon penuh yang sah) dianggap paid tanpa transaksi uang nol. Pembayaran dapat melunasi order saat masih diterima/diproses; itu tidak otomatis berarti cucian siap/diambil. Order cancelled tetap menyimpan ringkasan historis pembayaran, bukan tagihan baru.

## Uang, jumlah dan diskon

- `Money` adalah int64 **rupiah utuh**, 0..9.223.372.036.854.775.807; tidak ada float. Harga/order boleh nol, pembayaran dan posting ledger baru harus > 0. Arah kas diwakili tipe, bukan tanda negatif.
- `Quantity` berisi tepat salah satu dari `Pieces > 0` atau `Grams > 0`, sesuai satuan katalog; kg disimpan dalam gram. 2,5 kg = 2500 gram. Field alternatif wajib nol; jumlah nol/negatif/campuran ditolak.
- Tarif per piece: `harga × jumlah`. Tarif per kg: `harga × gram / 1000`, dibulatkan ke rupiah terdekat per baris, pecahan 0,5 ke atas. Tidak ada pembulatan berat/minimum kg tersembunyi. Konversi input kg desimal ke gram integer harus dilakukan secara eksak oleh DTO/boundary mendatang, bukan melalui float.
- Contoh: 7000 × 2500/1000 = Rp17.500. Tarif 7005/kg dengan 100 gram = Rp700,5 → Rp701. Dua baris masing-masing 0,5 rupiah menjadi subtotal Rp2; pembulatan dilakukan sebelum penjumlahan, bukan di akhir order.
- Subtotal menjumlahkan total baris dengan pemeriksaan overflow; total = subtotal − diskon. Perhitungan perkalian/penjumlahan menolak overflow int64, termasuk subtotal dari baris yang masing-masing masih valid. Kalkulasi gram menghindari overflow perantara ketika hasil akhir masih muat.
- Diskon hanya nominal per order: 0..subtotal. Diskon > 0 memerlukan alasan 1–250 karakter dan actor owner/admin aktif; ID pemberi persetujuan disimpan. Cashier tidak dapat memberi diskon sendiri. Diskon penuh sah dengan persetujuan; diskon negatif/melebihi subtotal ditolak, tidak di-clamp. Belum ada diskon item/persen/kupon/stacking.
- Pesanan berisi 1–100 item. Layanan harus ditemukan, valid dan aktif. ID katalog berulang ditolak. Item berulang diperbolehkan sebagai baris terpisah dan mengikuti pembulatan per baris.

`NewOrderInput` tidak menyediakan harga, line total, subtotal, total, paid amount atau status dari klien. `ItemRequest` hanya service ID dan quantity. **Service mendatang wajib mengambil customer/katalog/actor dari sumber tepercaya**, lalu memanggil `NewOrder`; jangan decode body HTTP menjadi `Service`/`Staff` dan mempercayainya. Actor diskon bukan data role yang dipilih frontend. Harga snapshot hasil domain yang disimpan, bukan total request.

## Validasi data dan waktu

- Nama customer/service/staf wajib 1–100 rune Unicode setelah trim; UTF-8 rusak, karakter kontrol atau format tersembunyi ditolak. Nama tidak dibatasi menjadi ASCII.
- Kontak boleh kosong. Jika diisi, separator spasi/tanda hubung/kurung dibuang; `0...` menjadi `+62...`, `62...` menjadi `+62...`; nomor internasional memakai `+` dengan 8–15 digit ASCII dan awal bukan nol. Huruf/ekstensi/newline/plus ganda ditolak. Ini validasi bentuk, bukan bukti kepemilikan/WhatsApp.
- Catatan customer/order opsional maksimal 1000 rune, trim dan normalisasi CRLF ke LF; newline/tab diizinkan, kontrol lain ditolak. Alasan diskon/pembatalan dan deskripsi ledger satu baris maksimal 250 rune. Tampilan HTML tetap wajib escaping pada boundary.
- ID/key 1–100 karakter ASCII alfanumerik/titik/underscore/hyphen. Username 3–50 karakter dari himpunan sama, case-sensitive, tanpa trim/normalisasi diam-diam pada akun lama. Unique username global ditangani DB.
- Created/updated time wajib, updated tidak boleh lebih awal. Estimasi order opsional saat input; jika kosong dihitung dari waktu penerimaan + durasi layanan terlama pada item. Override estimasi harus sesudah penerimaan; estimasi yang sudah terlewat saat order kemudian dibaca tetap sah sebagai order terlambat.

## Pembayaran, buku kas dan audit

`Order.RecordPayment` menerima pembayaran positif dalam cash/bank_transfer/qris yang dicatat manual. Menolak order berbeda, ID/key berulang dalam instance, waktu sebelum pembaruan terakhir, order ditutup, atau nominal melebihi sisa tagihan. DP parsial diperbolehkan; nominal adalah uang yang diterapkan ke tagihan, bukan uang tunai diserahkan sebelum kembalian. Belum ada kalkulasi kembalian, refund, payment reversal atau integrasi penyedia pembayaran.

`NewPaymentLedger` mengambil amount/order/payment/actor/time dari Payment, tidak menerima total ledger lain. Pengeluaran operasional memakai LedgerEntry KREDIT tanpa order/payment. Reference payment di fondasi ini hanya untuk penerimaan DEBIT dengan order; refund memerlukan rancangan transaksi terpisah, bukan nominal negatif atau penghapusan baris lama.

Penolakan duplicate domain sendiri adalah guard **in-memory**. Adapter memakai row lock dan unique key; service hari 07 memuat actor/order tepercaya, mengunci sebelum replay, memvalidasi pembayaran dan menyertakan audit hanya untuk posting baru pada transaksi yang sama. Runtime baru menggunakan boundary ini; data legacy tidak diimpor atau di-dual-write. Kegagalan audit membatalkan payment/ledger, dibuktikan melalui satu alur PostgreSQL terisolasi. Jangan memanggil generator ledger atas Payment yang belum diterima order.

Audit action yang tersedia: order_created, order_status_changed, payment_recorded, discount_approved, customer_updated, service_updated, staff_updated; masing-masing hanya cocok dengan jenis entity terkait. ID/time wajib. Append-only storage, audit saat commit, request correlation dan audit login/security masih pekerjaan service/audit berikutnya. Tidak ada janji domain memverifikasi eksistensi ID referensi tanpa repository.

## Error domain

`*domain.Error` memiliki `Kind`, `Code`, `Field`, `Message`; gunakan `errors.As`, termasuk setelah dibungkus `%w`. Pesan Indonesia statis tidak mengulang input, PII atau credential. Kind tersedia validation/conflict/forbidden/not_found; belum ada status HTTP dalam package.

| Kode | Makna |
|---|---|
| `invalid_value`, `money_overflow` | Input/invarian atau nilai uang tidak sah |
| `invalid_transition`, `order_closed` | Workflow tidak mengizinkan operasi |
| `service_inactive`, `duplicate_service`, `service_not_found` | Katalog tidak dapat dipakai |
| `discount_approval_required`, `staff_inactive` | Actor tidak diperbolehkan |
| `payment_required`, `payment_exists`, `overpayment`, `duplicate_payment` | Syarat pembayaran tidak terpenuhi |

Pemetaan Kind ke respons HTTP dilakukan di handler/boundary hari berikutnya; jangan mengirim object domain/credential langsung sebagai JSON atau menaruh error driver di error domain.

## Kompatibilitas dan pekerjaan berikutnya

- Handler/server baru memakai service/domain dan tidak memakai `models.Pesanan/Kasir` atau query legacy. `Sudah Lunas` hanya informasi finansial, tidak cukup untuk menentukan operational status. Label `Diproses`/`Selesai & Siap Diambil` dapat menjadi kandidat mapping processing/ready setelah inventaris; status asing dan uang historis perlu review, bukan konversi otomatis.
- Migration 000003 hari 06 menyediakan tabel domain BIGINT/ID varchar(100)/snapshot; tabel legacy tetap int4/varchar(50), tanpa konversi otomatis. Sebelum wiring wajib siapkan import/cutover yang menjaga data dan satu sumber transaksi; jangan cast int64 ke int4/Go int tanpa pemeriksaan atau menyalin data ke dua ledger.
- Rehidrasi memvalidasi total snapshot, chronology/payment state dan reference ID; use case kini memiliki auth intent, clock/ID/token injection dan atomic audit. Test fake/HTTP dan satu alur DB sudah ada; acceptance UI, session/user lifecycle dan import/cutover tetap perlu bukti terpisah.
- Uji domain: `go test ./internal/domain -count=1 -timeout=45s`. Semua fixture/clock eksplisit dan sintetis; tanpa `.env`, network, PostgreSQL, HTTP atau filesystem. Unit test hari 05 tidak membuktikan API lama aman, DB concurrency/idempotensi, atau readiness produksi.
