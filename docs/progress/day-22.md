# Laporan Hari 22 — Dashboard, Shift dan Laporan

## Tindakan — Hasil

- Dashboard sesuai peran — owner/admin mendapat ringkasan global, metode pembayaran, layanan populer dan detail laporan; kasir mendapat saldo/angka shift sendiri serta pintasan operasional tanpa request laporan global.
- Periode/angka — filter tanggal awal/akhir WITA inklusif, batas 366 hari, delapan summary cards, jenis/status order dan dua tabel detail berhalaman 20 baris tersedia. Piutang/antrean saat pembacaan dipisahkan dari transaksi periode; nominal nol, net negatif dan selisih negatif didukung. Integer tidak aman atau metadata tidak sesuai ditolak sebelum bagian ditampilkan.
- Shift — buka/mutasi/tutup kas sendiri, harian own/all, cards saldo/penerimaan/mutasi, transaksi berhalaman dan selisih penutupan diperjelas. Nilai seluruh sesi lintas tengah malam dibedakan dari transaksi tanggal laporan; tanpa shift dan metode historis diberi penjelasan.
- Retry shift — double-submit dikunci; hasil tidak pasti mempertahankan satu path/key/body untuk retry eksplisit. Perubahan isian dan penolakan setelah hasil tidak pasti tidak menghasilkan key pengganti. Hasil idempotent menampilkan ID; kegagalan refresh tidak mengubah transaksi terkonfirmasi menjadi gagal.
- CSV/cetak — ekspor hanya halaman detail yang dimuat, melalui endpoint privat existing dan pemeriksaan Content-Type/metadata. Byte CSV/proteksi formula dari server dipertahankan. Print A4 landscape menyertakan metadata serta cakupan halaman, menyembunyikan shell/form/controls dan mempertahankan header tabel.
- Tema/dokumentasi — palet dan tokens existing dipertahankan, tanpa grafik/framework/library baru. Reports, UI, README, arsitektur dan backlog diperbarui dalam tindakan/hasil. Tidak ada perubahan Go, query, API, migration atau dependency; schema tetap 12. Tidak ada commit/push/deployment.

## Verifikasi

- Focused Edge headless/API fixture sintetis — 25 order, penerimaan Rp100.000, refund Rp3.000, net Rp97.000, piutang Rp150.000 dan selisih −Rp1.000 tampil sesuai respons terkontrol. Net −Rp3.000, Rp0, cancelled dengan due nol, overflow dan periode kosong diperiksa; ini bukti UI, bukan rekonsiliasi database.
- Role/date/detail — owner/admin dapat membaca laporan; kasir tidak meminta laporan global, termasuk submit/ekspor paksa. Shift sendiri/semua dan penolakan fixture shift asing lulus. Perangkat Los Angeles tetap mengirim batas kalender WITA yang benar; inklusivitas akhir, periode invalid/terlalu panjang, leap year, filter jenis/status, pagination 20/5 dan metode historis diperiksa.
- CSV browser — berkas unduhan identik dengan keluaran handler Go laporan existing memakai use case sintetis. Formula `=SUM(1,2)`, kutipan/koma, whitespace/tab/formula/newline, timestamp +08:00, header metadata/offset dan tidak adanya token diperiksa. Metadata salah tidak menghasilkan unduhan. Generator fixture sementara tidak mengakses database atau mengubah backend.
- Browser states/shift — loading, empty, gagal per bagian/retry, offline tanpa kiriman otomatis, 403 dan 401 lulus. Buka/mutasi/tutup, saldo/selisih, zero ditolak untuk mutasi, double-submit, respons hilang dan replay key/body identik serta 409 setelah hasil tidak pasti diperiksa. Tidak ada exception JavaScript atau console.error.
- Visual/print — screenshot dashboard/cards/detail/shift desktop/ponsel ditinjau. Viewport 1280/768/360/390/412px tanpa overflow halaman; Enter/fokus dan region tabel diperiksa. Preview print menyembunyikan controls dan mempertahankan metadata/halaman transaksi. PDF A4 landscape fixture dashboard 5 halaman dan shift 3 halaman diperiksa dimensi/jumlah halamannya.
- Syntax/diff — sepuluh HTML dan enam belas JavaScript diperiksa untuk syntax, UTF-8, aset lokal, ID unik, tokens dan scope header. Dokumen/tautan serta diff/status Git diperiksa; whitespace bersih. Tidak menjalankan test Go/PostgreSQL/contract suite/full suite karena query/API tidak berubah, sesuai scope verifikasi Hari 22.
- Kebersihan/chime — browser/server mock dihentikan dan source generator fixture sementara di repository dihapus. Chime diputar dua kali melalui MediaPlayer, masing-masing mencapai akhir 2,376 detik.

## Perubahan Penting

- `frontend/dashboard.html/js`, `reports.css`, `reports-model.js` — dashboard berdasarkan peran, tanggal WITA, validasi, summary/detail/CSV dan print.
- `frontend/shifts.html/js/css` — ringkasan sendiri/harian, detail, states, print dan intent retry yang dibekukan.
- `frontend/ui/ui.js` — navigasi Dashboard tersedia untuk kasir dengan konten pribadi.
- [reports.md](../reports.md), [ui.md](../ui.md), README, arsitektur dan backlog — kontrak UI, bukti, batas snapshot/ekspor dan status Hari 22.

## Risiko atau Sisa Pekerjaan

- Browser memakai API mock; fixture CSV memakai handler asli dengan use case sintetis. Acceptance browser → server → PostgreSQL nyata, Android dan printer fisik belum diverifikasi pada Hari 22.
- Setiap bagian/halaman/CSV dibaca terpisah; transaksi baru dapat menggeser pagination dan angka. Cetak/ekspor hanya halaman yang dimuat, bukan seluruh hasil atau snapshot gabungan yang dibekukan.
- Key/body shift hanya dalam memori tab; reload/penutupan paksa/expiry dapat menghilangkannya. Periksa transaksi sebelum memasukkan ulang. Tidak ada antrean offline.
- Tidak ada blocker implementasi Hari 22 yang diketahui. PWA/offline shell tetap Hari 23; kesiapan produksi tidak diklaim.
