# Pembayaran, refund dan buku kas — Hari 14

Pembayaran dicatat manual melalui Go service dan PostgreSQL schema 10. Seluruh nominal memakai integer rupiah positif. Tidak ada payment gateway, pemindahan uang otomatis atau akuntansi double-entry. Kebijakan ini perlu ditinjau pemilik sebelum penggunaan operasional; database nyata belum dimigrasikan.

## Aturan pembayaran

- Owner/admin/cashier aktif dapat mencatat `cash`, `bank_transfer` atau `qris`. Server menentukan ID, order, pencatat dan waktu UTC dengan presisi mikrodetik. Record baru berstatus `posted`; status laundry tidak berubah.
- DP dan beberapa pembayaran pada satu order diperbolehkan. Nominal maksimal sisa tagihan; overpayment ditolak 409. Amount adalah rupiah yang diterapkan ke tagihan, bukan uang diserahkan sebelum kembalian. Order Rp0 langsung lunas dan tidak memiliki posting nol.
- Payment memiliki ID sendiri, order reference, method, amount, timestamp, cashier, status dan idempotency key. Status respons `refunded` diturunkan dari refund; receipt asli di database tetap `posted` dan tidak ditimpa.
- `Idempotency-Key` wajib, 1–100 karakter ASCII huruf/angka/titik/underscore/hyphen. Scope unik payment `(order_id,key)`, terpisah dari refund dan command order. Retry amount/method/actor yang sama mengembalikan ID semula dengan `replayed=true`, tanpa posting/audit/version baru. Perubahan payload atau actor pada key sama ditolak 409. Izin staf terkini selalu diperiksa.
- Payment baru pada order `collected`/`cancelled` ditolak. Replay sah tetap dapat dibaca setelah order ditutup atau pembayaran telah di-refund; key lama tidak dapat dipakai untuk membayar ulang.

## Aturan refund dan pembatalan

- Hanya owner/admin aktif dapat refund. Body berisi `payment_id` dan alasan satu baris 1–250 karakter; amount/method/actor/waktu dari klien ditolak. Server menyalin seluruh nominal dan metode pembayaran asal.
- Satu payment hanya dapat dikembalikan penuh satu kali. Refund sebagian, refund setelah pengambilan, penggantian metode refund dan koreksi nominal langsung belum tersedia. Refund sah ketika status order `received`, `processing` atau `ready`; status laundry tetap.
- Scope key refund `(order_id,key)`. Retry dengan payment/actor/alasan yang sama mengembalikan refund semula, termasuk setelah pembatalan; key sama dengan payload berbeda ditolak 409. Key baru untuk payment yang sudah di-refund juga ditolak 409.
- Saldo terbayar = jumlah payment yang belum di-refund; sisa tagihan = total − saldo terbayar. Order dapat dibayar ulang dengan key baru. Pengambilan tetap harus lunas. Pembatalan hanya dari `received`/`processing`, setelah seluruh pembayaran dikembalikan dan saldo terbayar nol. Refund dan cancel adalah dua perintah terpisah; jika cancel gagal, refund yang sudah commit tetap tercatat dan dapat ditinjau.
- Ini pencatatan manual: staf harus memastikan pengembalian uang telah dilakukan melalui metode asal. API tidak mengirim transfer/QRIS. Kegagalan jaringan ditangani dengan retry key/payload semula, bukan refund kedua.

## Transaksi, constraint dan laporan

Use case mengotorisasi actor, mengunci order `FOR UPDATE`, memeriksa replay dan invarian domain, lalu menyimpan payment/refund, satu ledger entry, version/update time dan audit dalam satu transaksi. Waktu penerimaan dipilih setelah lock. Kegagalan posting, audit atau commit menghasilkan error dan tidak mengembalikan objek sukses. Hasil commit yang tidak diketahui karena jaringan harus direkonsiliasi melalui retry.

Payment menghasilkan `DEBIT`, refund `KREDIT`, keduanya positif dan menyimpan metode masing-masing. Receipt/refund/ledger append-only; koreksi memakai record baru. UNIQUE key, UNIQUE ledger.payment_id/refund_id dan FK komposit order/amount/method melindungi referensi refund. Trigger financial mengunci order dan menolak saldo/chronology/status invalid; deferred constraint trigger menolak commit tanpa tepat satu posting dengan order, nominal, tipe, metode, actor dan waktu yang cocok. Audit bisnis dijamin melalui service transaction, bukan trigger pembuat audit otomatis.

Laporan `/api/laporan/neraca` menghitung seluruh metode, bukan saldo uang tunai laci: inflow = DEBIT, outflow = KREDIT termasuk refund dan pengeluaran operasional, balance = inflow − outflow pada periode `[from,to)`. Refund periode lain menjadi outflow pada tanggal refund, tanpa menulis ulang penerimaan lama. Contoh Rp3.000 cash + Rp2.000 QRIS, refund cash Rp3.000, lalu pembayaran Rp8.000 transfer: inflow Rp13.000, outflow Rp3.000, net Rp10.000. Shift/rekonsiliasi fisik dan laporan terpisah per metode masih tahap berikutnya; pengeluaran historis tanpa metode tetap NULL.

Role runtime membutuhkan SELECT/INSERT payments/refunds/ledger_entries, SELECT/UPDATE orders dan hak audit existing. Jangan berikan UPDATE/DELETE/TRUNCATE record finansial, DDL, kepemilikan tabel atau superuser. Trigger tidak melindungi dari pemilik schema yang sengaja menonaktifkannya. Payment/refund history privat dan no-store, tanpa gateway credential atau data bank sensitif.

## Migration dan verifikasi

Jalankan runner resmi `go run ./cmd/migrate status`, `go run ./cmd/migrate up`, lalu `status`. Backup/restore salinan dan hentikan writer sebelum upgrade nyata. Versi 10 menambah refund, method ledger dan guard finansial; backfill metode dari payment asal, tanpa mengarang refund. Receipt tanpa ledger cocok, key historis invalid atau timestamp nonfinite menyebabkan migration gagal atomik; rekonsiliasi harus ditinjau, jangan menonaktifkan constraint atau menghapus data. SQL 1–9 tetap immutable. Tidak ada down otomatis; pertahankan schema 10 dan gunakan forward fix/binary kompatibel. Writer sebelum schema 10 dapat menghitung saldo refund secara salah dan tidak boleh dijalankan kembali. Lihat [database.md](database.md).

Gunakan `ULAUNDRY_TEST_ADMIN_DSN` hanya ke database `postgres` pada cluster test khusus. Test membuat/menghapus database disposable dan tidak membaca `.env`:

```powershell
go test ./internal/domain ./internal/service ./internal/http/... -skip '^TestPostgres' -count=1 -timeout=60s
go test ./internal/service -run '^TestPostgres(FinancePaymentsAndRefunds|PaymentUseCaseAtomicity|OrderConcurrency)$' -count=1 -timeout=120s -v
go test ./internal/migrate -run '^TestPostgresFinanceUpgrade$' -count=1 -timeout=90s -v
```

Bukti mencakup DP/metode campuran, overpayment, refund/retry, delapan request identik, dua key bersaing, refund versus pickup, rollback ledger/audit, constraint serta upgrade 9→10/penolakan data invalid. Race instrumentation perlu host dengan compiler C kompatibel; bukti concurrency biasa bukan bukti `-race`. Kontrak HTTP ada di [api.md](api.md), hasil aktual di [laporan hari 14](progress/day-14.md).
