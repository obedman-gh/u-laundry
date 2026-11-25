# Laporan Hari 20 — UI POS dan Pembuatan Pesanan

## Tindakan — Hasil

- POS kasir — `/pos.html` tersedia dari navigasi dan antrean untuk seluruh staf aktif. Cari/pilih pelanggan, create customer minimal, layanan multi-baris kg/item, harga/subtotal, diskon owner/admin, estimasi WITA, catatan, submit serta hasil/tautan struk memakai API existing.
- Harga/total — perkiraan katalog memakai gram/integer dan BigInt dengan pembulatan half-up per baris; payload tidak mengirim harga/subtotal/total/status. Hasil menampilkan snapshot server dan memberi pemberitahuan bila harga final berubah. Minimum 1 gram/1 item dan total Rp0 tetap didukung; pembayaran tidak diposting oleh create order.
- Idempotency — satu key acak kriptografis dan body JSON tetap dibuat sebelum submit. Double-click dikunci; network/timeout/5xx/respons tidak terbaca mengunci form untuk pemeriksaan/retry eksplisit dengan key/body sama. Replay tidak membuat order baru atau mengarang token tracking. Penolakan setelah percobaan tidak pasti tidak membuka form untuk order pengganti.
- Customer/privasi — nama/kontak dirender dengan createElement/textContent, tanpa innerHTML. Pelanggan tanpa kontak didukung; create ganda dikunci. API create customer belum idempotent: hasil tidak pasti harus dicari sebelum create diizinkan kembali, dan pilihan pelanggan lama dibersihkan saat create baru dikirim.
- UX — validasi per kolom/fokus, loading/error/success/offline, hasil kosong, kontrol keyboard, ringkasan desktop dan layout ponsel memakai palet/tokens existing. Tidak ada antrean offline atau retry otomatis saat koneksi kembali. Hasil menyediakan ID, rincian harga/kuantitas, total final, status, estimasi, struk privat dan tracking fragment bila tersedia.
- Dokumentasi — docs/ui.md mencatat tindakan — hasil, state/retry serta batas draft memori; README/API/arsitektur/backlog diperbarui. Tidak ada perubahan Go/domain/endpoint/migration/dependency; schema tetap 12. Tahap berikutnya Hari 21 — antrean/detail/progres/receipt.

## Verifikasi

- `node --test scripts/frontend-contract.test.cjs` — 29 test lulus. Tujuh tambahan POS mencakup konversi gram tanpa float, half-up per baris, batas integer aman, payload tanpa harga/total klien, diskon sesuai role/alasan/batas, estimasi WITA, snapshot final server dan input tidak lengkap/terlalu banyak baris.
- Edge headless/API mock lokal — create campuran 100 gram × Rp7.005/kg dan dua item × Rp20.000 menghasilkan Rp40.701 dari server. Double-submit menghasilkan satu POST/satu order. Struk dari tautan hasil berhasil dibuka pada tab terpisah dengan data mock.
- Browser retry — commit diikuti JSON tidak terbaca dan AbortController dengan timer pengujian dipercepat tetap memulihkan order melalui key/body identik tanpa order tambahan. Navigasi tab yang sama dan klik Keluar ditahan selama hasil tidak pasti. Perubahan harga server menjadi Rp8.000/kg ditampilkan sebagai total final Rp800, berbeda dari perkiraan Rp701. Penolakan 409 menyediakan perbaikan; 503 lalu 403 tetap mengunci intent sampai hasil terkonfirmasi.
- Browser state/izin — offline nyata melalui emulasi jaringan tidak mengirim/mengantrekan order dan reconnect tidak mengirim otomatis. Customer create/double-submit/hasil tidak pasti, markup nama aman, customer tanpa kontak, diskon kasir melalui manipulasi DOM ditolak, diskon owner/total nol, estimasi WITA, katalog kosong/gagal/retry dan expiry 401 tanpa loop/dialog draft lulus.
- Browser keyboard/mobile — Tab/Enter pencarian dan pemilihan pelanggan, fokus layanan/validasi/retry/hasil, hapus baris, serta viewport 1280/768/360/390/412px tanpa overflow diperiksa. Screenshot draft/hasil desktop dan form/hasil mobile ditinjau; palet tetap. Tidak ada exception JavaScript atau console.error; HTTP gagal sengaja disimulasikan.
- Syntax/aset — sepuluh HTML dan tiga belas JavaScript diperiksa untuk syntax, UTF-8, referensi lokal, ID statis unik, CSS tokens terdefinisi dan scope header tabel. Dokumentasi/tautan, diff dan status Git diperiksa; `git diff --check` bersih.
- Cakupan — tidak menambah/menjalankan test handler/use-case atau full backend/database suite karena perilaku backend tidak berubah. Tidak mengakses `.env`, PostgreSQL atau data operasional; harness/screenshot sintetis berada di cache luar repository dan browser/server mock dihentikan setelah verifikasi. Tidak ada commit/push/deployment oleh pekerjaan ini.
- Chime — chime-1.mp3 diputar dua kali melalui MediaPlayer .NET; masing-masing mencapai posisi akhir 2,376 detik.

## Perubahan Penting

- `frontend/pos.html`, `pos.css`, `pos.js` — alur POS responsif, customer/catalog, lifecycle command, retry aman dan hasil/receipt.
- `frontend/pos-model.js` — validasi batas, konversi kg/gram, perkiraan BigInt, WITA serta pemeriksaan bentuk/total respons server; bukan sumber harga final.
- `frontend/index.html`, `frontend/ui/ui.js` — pintasan dan navigasi Pesanan baru sesuai role staf.
- `scripts/frontend-contract.test.cjs` — tujuh test POS ditambahkan ke command frontend resmi, total 29.
- [ui.md](../ui.md), README, API, architecture dan backlog — kontrak UI, evidence, batas retry/draft dan status tahap 20.

## Risiko atau Sisa Pekerjaan

- Key/body/draft hanya bertahan di memori tab. Navigasi pending dibatasi dan beforeunload memberi peringatan, tetapi reload/penutupan paksa tetap dapat menghilangkannya; periksa antrean sebelum memasukkan ulang. Customer creation belum idempotent pada API.
- Sebelum submit harga adalah perkiraan katalog; total final dikonfirmasi saat server menyimpan order. Tidak ada quote/reservasi harga baru. Nominal di luar integer aman JavaScript ditolak UI meskipun backend mendukung int64.
- Verifikasi browser memakai API mock; konkurensi/idempotency PostgreSQL nyata, UI–database, printer dan Android fisik belum diverifikasi pada Hari 20. Tautan struk memakai endpoint existing; penyempurnaan receipt/cetak dan antrean/progres tetap Hari 21. Tidak ada blocker implementasi Hari 20 yang diketahui.
