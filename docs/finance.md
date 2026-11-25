# Pembayaran, refund, shift dan buku kas — Hari 15

Pembayaran dicatat manual melalui Go service dan PostgreSQL schema 11. Seluruh nominal memakai integer rupiah; penerimaan/pengeluaran positif, saldo awal/fisik boleh nol, selisih boleh negatif. Tidak ada payment gateway, pemindahan uang otomatis atau akuntansi double-entry. Kebijakan ini perlu ditinjau pemilik sebelum penggunaan operasional; database nyata belum dimigrasikan.

## Aturan pembayaran

- Owner/admin/cashier aktif dapat mencatat `cash`, `bank_transfer` atau `qris`. Server menentukan ID, order, pencatat dan waktu UTC dengan presisi mikrodetik. Record baru berstatus `posted`; status laundry tidak berubah.
- DP dan beberapa pembayaran pada satu order diperbolehkan. Nominal maksimal sisa tagihan; overpayment ditolak 409. Amount adalah rupiah yang diterapkan ke tagihan, bukan uang diserahkan sebelum kembalian. Order Rp0 langsung lunas dan tidak memiliki posting nol.
- Payment memiliki ID sendiri, order reference, method, amount, timestamp, cashier, status dan idempotency key. Status respons `refunded` diturunkan dari refund; receipt asli di database tetap `posted` dan tidak ditimpa.
- `Idempotency-Key` wajib, 1–100 karakter ASCII huruf/angka/titik/underscore/hyphen. Scope unik payment `(order_id,key)`, terpisah dari refund dan command order. Retry amount/method/actor yang sama mengembalikan ID semula dengan `replayed=true`, tanpa posting/audit/version baru. Perubahan payload atau actor pada key sama ditolak 409. Izin staf terkini selalu diperiksa.
- Payment baru pada order `collected`/`cancelled` ditolak. Replay sah tetap dapat dibaca setelah order ditutup atau pembayaran telah di-refund; key lama tidak dapat dipakai untuk membayar ulang.
- Sejak schema 11, payment dan refund tunai baru wajib shift aktif milik pencatat. Non-tunai otomatis terkait shift aktif bila ada; tanpa shift tetap sah dan masuk rekonsiliasi harian bagian tanpa shift. Respons receipt menyertakan shift_id jika terkait; sumber relasinya ledger append-only.

## Aturan refund dan pembatalan

- Hanya owner/admin aktif dapat refund. Body berisi `payment_id` dan alasan satu baris 1–250 karakter; amount/method/actor/waktu dari klien ditolak. Server menyalin seluruh nominal dan metode pembayaran asal.
- Satu payment hanya dapat dikembalikan penuh satu kali. Refund sebagian, refund setelah pengambilan, penggantian metode refund dan koreksi nominal langsung belum tersedia. Refund sah ketika status order `received`, `processing` atau `ready`; status laundry tetap.
- Scope key refund `(order_id,key)`. Retry dengan payment/actor/alasan yang sama mengembalikan refund semula, termasuk setelah pembatalan; key sama dengan payload berbeda ditolak 409. Key baru untuk payment yang sudah di-refund juga ditolak 409.
- Saldo terbayar = jumlah payment yang belum di-refund; sisa tagihan = total − saldo terbayar. Order dapat dibayar ulang dengan key baru. Pengambilan tetap harus lunas. Pembatalan hanya dari `received`/`processing`, setelah seluruh pembayaran dikembalikan dan saldo terbayar nol. Refund dan cancel adalah dua perintah terpisah; jika cancel gagal, refund yang sudah commit tetap tercatat dan dapat ditinjau.
- Ini pencatatan manual: staf harus memastikan pengembalian uang telah dilakukan melalui metode asal. API tidak mengirim transfer/QRIS. Kegagalan jaringan ditangani dengan retry key/payload semula, bukan refund kedua.

## Transaksi, constraint dan laporan

Use case mengotorisasi actor, mengunci order `FOR UPDATE`, memeriksa replay dan invarian domain, lalu menyimpan payment/refund, satu ledger entry, version/update time dan audit dalam satu transaksi. Waktu penerimaan dipilih setelah lock. Kegagalan posting, audit atau commit menghasilkan error dan tidak mengembalikan objek sukses. Hasil commit yang tidak diketahui karena jaringan harus direkonsiliasi melalui retry.

Payment menghasilkan `DEBIT`, refund `KREDIT`, keduanya positif dan menyimpan metode masing-masing. Receipt/refund/ledger append-only; koreksi memakai record baru. UNIQUE key, UNIQUE ledger.payment_id/refund_id dan FK komposit order/amount/method melindungi referensi refund. Trigger financial mengunci order dan menolak saldo/chronology/status invalid; deferred constraint trigger menolak commit tanpa tepat satu posting dengan order, nominal, tipe, metode, actor dan waktu yang cocok. Audit bisnis dijamin melalui service transaction, bukan trigger pembuat audit otomatis.

Laporan `/api/laporan/neraca` menghitung seluruh metode, bukan saldo uang tunai laci: inflow = DEBIT, outflow = KREDIT termasuk refund dan pengeluaran operasional, balance = inflow − outflow pada periode `[from,to)`. Sejak hari 15, cash in/out manual juga termasuk arus ledger ini; jangan menganggapnya omzet. Refund periode lain menjadi outflow pada tanggal refund, tanpa menulis ulang penerimaan lama. Contoh Rp3.000 cash + Rp2.000 QRIS, refund cash Rp3.000, lalu pembayaran Rp8.000 transfer: inflow Rp13.000, outflow Rp3.000, net Rp10.000 sebelum mutasi kas lain. Ringkasan shift memisahkan penerimaan/refund per metode dan mutasi kas; pengeluaran historis tanpa metode tetap NULL dan tampil sebagai metode belum dicatat.

## Shift kasir dan rekonsiliasi — Hari 15

- Satu shift aktif per staf, ditegakkan partial UNIQUE index. Owner/admin/cashier aktif membuka, mencatat cash in/out, dan menutup **shift sendiri**. Owner/admin dapat membaca ringkasan dan transaksi seluruh staf; cashier hanya miliknya. Actor/shift tidak dapat dipilih bebas dari body. Tidak ada force-close, edit/reopen shift tertutup atau transfer otomatis antar-kasir. Model mengasumsikan kas yang dipertanggungjawabkan terpisah per staf; laci bersama memerlukan serah terima yang ditinjau pemilik.
- Buka kas menerima opening ≥0 dan catatan opsional satu baris maksimal 250 karakter. Saldo awal adalah uang yang benar-benar ada saat mulai, tidak mem-posting pendapatan/ledger. Tidak otomatis membawa saldo shift lama. Server menentukan ID/cashier/waktu. Shift boleh melewati tengah malam tanpa penutupan otomatis.
- Cash in/out menerima amount >0, direction in/out dan alasan wajib satu baris maksimal 250 karakter. Setiap mutasi membuat satu ledger DEBIT/KREDIT, receipt key dan audit atomik. Ini dapat dipakai untuk tambahan uang kecil atau kas keluar operasional; penamaan/alasan harus menjelaskan tujuan. Tidak ada nominal negatif, perubahan record lama atau posting uang tanpa alasan.
- Saldo sistem tunai = opening + cash_payments − cash_refunds + cash_in − cash_out. Transfer/QRIS serta metode historis tak diketahui tidak mengubah laci. Perhitungan int64 memeriksa overflow; cash out/refund tidak boleh membuat saldo sistem negatif. Kekurangan uang untuk refund memerlukan tambahan kas yang benar-benar diterima sebelum dicatat.
- Refund tunai memakai shift aktif **pencatat refund**, walaupun pembayaran asal ada pada shift lama/kasir lain. Hanya owner/admin dapat refund sesuai aturan sebelumnya. Penutupan lama tetap; penerimaan lama tidak dipindahkan/dihapus. Replay payment/refund yang sah tetap mengembalikan shift asal setelah close, tanpa membutuhkan shift baru.
- Tutup kas menerima physical ≥0 dan catatan; server menghitung system dan difference = physical − system. Catatan wajib bila ada selisih, baik kurang maupun lebih. closed_at/closed_by/physical/system/difference/catatan disimpan satu kali dengan audit. Penutupan tidak mem-posting penerimaan, pengeluaran atau koreksi untuk memaksa selisih nol. Selisih adalah informasi pemeriksaan owner, bukan transaksi fiktif.
- Semua command shift memakai tepat satu Idempotency-Key. Scope buka `(cashier_id,key)`, mutasi `(shift_id,key)`, tutup satu receipt per shift. Retry payload/actor sama mengembalikan hasil existing; key/payload berbeda konflik. Retry buka shift lama yang sudah ditutup mengembalikan shift tertutup, tidak membuka ulang atau mengganti shift aktif baru. Izin terkini selalu diperiksa. UI menyimpan key retry dalam memori sampai hasil pasti; tidak menyimpan cookie/token/payload ke localStorage dan tidak mengantrekan mutasi offline.
- Lock: order → shift untuk payment/refund; penutupan/mutasi hanya mengunci shift. Buka memakai advisory lock per pencatat dan UNIQUE index. Waktu posting baru dipilih setelah lock. Penutupan memeriksa seluruh posting yang sudah commit; transaksi yang kalah dari close tidak dapat menambah posting pada shift tertutup. Audit gagal membatalkan shift/mutasi/penutupan dan key-nya. Trigger juga memeriksa pemilik, saldo, chronology dan snapshot penutupan terhadap ledger.

Halaman `/shifts.html` menyediakan buka/mutasi/tutup kas sendiri, pilihan tanggal, transaksi terkait dengan pagination, dan cetak ringkasan A4 landscape. Tema biru existing dipertahankan; tabel pada ponsel digeser agar nominal terbaca. Laporan harian memakai `[00:00,00:00 berikutnya)` WITA untuk transaksi; daftar sesi mencakup shift yang beririsan dengan hari tersebut (termasuk shift semalam). Baris setiap sesi memakai total **sepanjang shift**, sehingga tidak dijumlahkan ulang sebagai transaksi harian. Metode non-tunai ditampilkan masuk/refund/net dan bagian tanpa shift untuk dibandingkan dengan bukti bank/QRIS secara manual; tidak ada konfirmasi settlement otomatis. Transaksi historis tetap tanpa shift dan tidak mengubah saldo laci baru.

Hak runtime tambahan: SELECT/INSERT cash_shifts, UPDATE hanya kolom penutupan; SELECT/INSERT shift_cash_movements dan ledger_entries. Tanpa DELETE/TRUNCATE/DDL atau kepemilikan tabel. Trigger menolak edit saldo awal, reopen, perubahan shift tertutup, posting pada shift asing/tertutup dan mutasi tanpa receipt key. Staf nonaktif tidak dapat menutup sendiri; pemulihan akses memakai prosedur pengelolaan staf existing oleh owner, bukan bypass izin/force-close.

Upgrade ke schema 11 wajib menghentikan writer dan memverifikasi backup/restore salinan dahulu. Migration tidak membuat shift atau mengisi shift_id historis; SQL 1–10 tetap immutable. Tidak ada down otomatis. Binary schema 10 tidak menyertakan relasi shift pada posting tunai dan akan ditolak trigger; gunakan binary kompatibel schema 11 atau forward fix, jangan menonaktifkan guard. Detail di [database.md](database.md).

Verifikasi terarah tambahan, dengan DSN cluster test khusus yang sama:

```powershell
go test ./internal/domain -run '^TestCashShiftReconciliation$' -count=1 -timeout=45s
go test ./internal/service -run '^TestPostgresCashShifts$' -count=1 -timeout=120s -v
go test ./internal/migrate -run '^TestPostgresShiftUpgrade$' -count=1 -timeout=60s -v
go test ./internal/http/... ./internal/app -count=1 -timeout=60s
node --check frontend/shifts.js
```

Test PostgreSQL shift mencakup open/close/replay, duplicate active, asosiasi cash/noncash/refund, cash shortage, snapshot lintas shift, saldo/selisih, WITA/overnight, ownership, concurrency, constraint dan rollback audit. Bukti aktual browser/API/DB/cetak dan batas race/printer mengikuti [laporan hari 15](progress/day-15.md).

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
