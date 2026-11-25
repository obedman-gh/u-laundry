# Laporan Hari 21 — UI Antrean, Transaksi dan Receipt

## Tindakan — Hasil

- Antrean — search global ID/nama snapshot, lima status operasional, pagination 20 pesanan dan indikator terlambat tersedia pada `/index.html`. Search/status diproses bersama sebelum pagination PostgreSQL; `%`/`_` literal, nomor tidak dinormalisasi menjadi kontak.
- Detail/aksi — item dan harga snapshot, subtotal/diskon/total, saldo server, versi, estimasi WITA, milestone, catatan dan riwayat pembayaran berhalaman tersedia. Pengambilan wajib lunas; pembatalan received/processing tanpa saldo serta refund penuh per pembayaran posted hanya owner/admin. Cashier tetap dapat mencatat pembayaran.
- Konflik — stale version/409 menonaktifkan mutasi sampai refresh dan peninjauan ulang. Refresh tidak mengirim intent lama dengan versi baru; hasil kiriman sukses juga dikunci sampai snapshot dimuat kembali. Kartu antrean diberi pemberitahuan ketika perlu diperbarui.
- Payment/refund — double-submit dikunci; satu key acak dan body tetap dipertahankan saat timeout/network/408/5xx/respons tidak terbaca. Retry eksplisit mengirim byte yang sama dan menampilkan ID/nominal/hasil idempotent. Penolakan sesudah hasil tidak pasti tetap menahan intent; tidak ada antrean offline atau retry otomatis.
- Receipt/privasi — struk privat 58/80 mm memakai html/template, rincian layanan, saldo dan WITA. Kontrol cetak tersembunyi saat print; data pelanggan/layanan/catatan di-escape. Receipt tetap membutuhkan staf aktif berwenang dan tracking publik tetap memakai token dengan projection minimal.
- Dokumentasi — panduan tindakan/hasil, API tambahan, batas konsistensi/retry, checklist dan command verifikasi diperbarui. Tidak ada dependency, perubahan transaksi status/payment/refund atau migration; schema tetap 12. Tidak ada commit, push atau deployment.

## Verifikasi

- `node --test --test-name-pattern=queue scripts/frontend-contract.test.cjs` — tiga test lulus: graph tindakan/izin, batas nominal/presisi, overdue dan snapshot invalid. Tiga puluh satu test tersedia pada script resmi; test lain tidak dijalankan.
- HTTP terarah pada `internal/http/handler` dan `internal/http` — lulus untuk search/query invalid, saldo detail/customer history, pilihan kertas, escaping pelanggan/layanan/catatan panjang, no-store, receipt anonim/role asing, token tracking serta kontrak key status/payment. Filter command lengkap ada di [ui.md](../ui.md#verifikasi-hari-21--tindakan--hasil).
- `go test ./internal/service -run '^TestPostgres(QueueSearch|OrderConcurrency|FinancePaymentsAndRefunds)$' -count=1 -timeout=120s -v` — tiga suite lulus, termasuk 17 subtest workflow/finance. Cluster PostgreSQL 16 test baru di loopback 55451, DB disposable dibuat/dihapus helper. Bukti mencakup pencarian literal/pagination gabungan, stale version, concurrent duplicate/replay, competing keys, payment versus cancel/pickup, refund dan rollback ledger/audit; tanpa DB operasional atau `.env`.
- Edge headless/API mock sintetis — lima filter, pagination/kosong, overdue, safe DOM, detail, conflict/refresh, payment double-submit, lost response/timeout dan replay byte-identik, 403 setelah hasil tidak pasti, refund permission/replay, cancel dan pelunasan sebelum pickup lulus. Offline tidak mengantre/mengirim otomatis, gagal muat/retry, token publik dan expiry 401 tanpa dialog terjebak diperiksa. Tidak ada exception JavaScript atau console.error.
- Browser/mobile/print — keyboard Enter/fokus detail dan viewport 1280/768/360/390/412px tanpa overflow lulus; screenshot antrean/detail desktop/ponsel ditinjau. HTML receipt asli dari handler test memakai CSS/script nyata: preview print 58/80 mm terbaca, tombol tersembunyi, nama tanpa spasi/catatan panjang terbungkus dan markup tetap teks. PDF kedua lebar diperiksa dimensi/jumlah halaman: receipt biasa satu halaman, fixture panjang dua halaman pada tinggi kertas 200 mm. Printer fisik tidak diuji.
- Syntax/aset/build — sepuluh HTML dan lima belas JavaScript diperiksa untuk syntax, UTF-8, aset lokal, ID unik, tokens dan header tabel. Build `cmd/server` berhasil; artefak berada di cache luar repository. Diff/status Git ditinjau dan `git diff --check` bersih. Tidak menjalankan full suite, race atau release checks yang tidak terkait.
- Kebersihan pengujian — browser/server mock dan cluster PostgreSQL test Hari 21 dihentikan; dokumentasi UTF-8/tautan lokal serta empat bagian laporan diperiksa. Chime diputar dua kali melalui MediaPlayer; masing-masing mencapai akhir 2,376 detik.

## Perubahan Penting

- `frontend/index.html`, `queue.js/css`, `queue-model.js` — antrean/detail responsif, permission dan lifecycle mutasi/retry.
- `internal/domain/order.go`, service/repository order — search tervalidasi dan SQL berparameter sebelum pagination.
- DTO/handler order/customer — tambahan saldo integer dan catatan detail; history customer menjaga projection saldo yang sama.
- `internal/http/handler/receipt.go`, `frontend/receipt.js/css` — template struk privat escaped dan cetak thermal dengan pilihan 58/80 mm.
- Test frontend/HTTP/PostgreSQL — bukti terarah; README, API, arsitektur, backlog dan [ui.md](../ui.md) mencatat kontrak serta batas implementasi.

## Risiko atau Sisa Pekerjaan

- Key/body pending hanya dalam memori tab. Reload/penutupan paksa/expiry dapat menghilangkannya; periksa riwayat order dan pembayaran sebelum memasukkan ulang. Navigasi normal ditahan selama hasil belum pasti.
- Browser memakai API mock; PostgreSQL diverifikasi terpisah, belum acceptance browser → server → database nyata. Android, printer dan driver thermal fisik belum diperiksa; ukuran driver/skala/header-footer perlu dicocokkan sebelum penggunaan operasional.
- Offset pagination dapat bergeser saat ada writer lain; detail/history juga GET terpisah. Server tetap menegakkan saldo/status/izin di bawah lock dan version untuk status/cancel. Search substring belum memakai indeks khusus; ukur dahulu bila volume data meningkat.
- Tidak ada blocker implementasi Hari 21 yang diketahui. UI laporan lengkap tetap Hari 22; tugas ini tidak menyatakan kesiapan produksi.
