# Laporan Hari 05 — Model Domain dan Aturan Operasional Laundry

## Tindakan — Hasil

- Pembentukan `internal/domain` — delapan model Customer, Service, Order, OrderItem, Payment, LedgerEntry, Staff/User dan AuditEvent, tanpa dependency HTTP/PostgreSQL atau dependency baru.
- Pemisahan workflow — lima status operasional dengan tabel transisi eksplisit, milestone dan cancellation guard; status unpaid/partially_paid/paid diturunkan dari pembayaran, tanpa posting uang saat status laundry berubah.
- Kalkulasi terpusat — integer rupiah int64, gram/jumlah integer, rounding half-up per baris, checked arithmetic, snapshot harga katalog, subtotal/diskon/total dihitung domain; input order tidak menerima total/harga/status klien.
- Validasi dan error bertipe — nama/kontak/layanan/quantity/harga/diskon/estimasi/catatan, actor aktif, reference/payment/audit divalidasi; `domain.Error` menyediakan Kind/Code/Field/Message tanpa mengulang data sensitif.
- Dokumentasi aturan — pilihan awal diskon nominal owner/admin, DP, pickup lunas, pembatalan dan rounding dicatat untuk ditinjau sebelum aktivasi; kode/schema/data hari 04 dipertahankan.

## Verifikasi

- `go test ./internal/domain -count=1 -timeout=45s -cover` — lulus; coverage 93,5% statement pada package domain saja. Mencakup 25 pasangan transisi, status asing, total/discount/snapshot, validasi dan kegagalan tanpa mutasi.
- Referensi aritmetika `math/big` pada 49 kombinasi harga/gram — hasil rounding/overflow sesuai, termasuk harga/bobot ekstrem dan intermediate multiplication yang melebihi int64 saat hasil akhir masih muat.
- Unit payment/ledger/audit — parsial/lunas, overpayment/duplicate, waktu tidak sah, cancel setelah bayar, pickup belum lunas, pembayaran order ditutup dan kecocokan aksi audit terverifikasi menggunakan data/waktu sintetis.
- `gofmt`, `go vet ./internal/domain`, review import/domain, tautan dokumen serta Git diff/status — selesai; perubahan tidak menyentuh handler, SQL, dependency atau UI. Tidak menjalankan integration PostgreSQL, UI, full suite atau release checks.
- Chime `sounds/chime-1.mp3` — playback selesai dua kali.

## Perubahan Penting

- `internal/domain/` — model dan aturan bisnis murni, constructor/validasi, workflow/uang dan targeted unit tests.
- `docs/business-rules.md` — tabel status, formula/rounding, batas input, payment/ledger/audit, error contract dan rencana kompatibilitas.
- `README.md`, `docs/target-architecture.md`, `docs/backlog-30-days.md` — status fondasi domain dan command test terarah diperbarui; hasil hari 04 tetap tersedia.

## Risiko atau Sisa Pekerjaan

- Domain belum dihubungkan ke HTTP/service/repository. Handler lama masih mempercayai total frontend dan mencampur status/pembayaran; guard domain hari 05 belum memperbaiki alur aktif atau menutup risiko baseline.
- Schema lama int4/tiga tabel/ID pesanan varchar(50) belum menyimpan seluruh model int64/snapshot; perlu migration additive atau pemeriksaan batas adapter sebelum wiring. Tidak ada migrasi data otomatis atau rehidrasi order hari ini.
- Pembulatan, diskon, DP, pelunasan sebelum pengambilan dan cancellation guard merupakan pilihan awal yang perlu ditinjau sebelum diaktifkan. Refund/kredit pelanggan/promo/minimum kg belum diimplementasikan.
- Duplicate guard hanya in-memory; uniqueness dan replay idempotency, transaksi payment-ledger-audit, locking, otorisasi actor dan penyimpanan audit append-only masih memerlukan service/repository beserta bukti database. Aplikasi belum layak produksi.
