# Laporan Hari 18 — Fondasi UI Responsif dan Tokens

## Tindakan — Hasil

- Tampilan existing — baseline login/antrean ditinjau; biru #007bff/#0069d9/#0056b3 dan login #2563eb/#1d4ed8 dipertahankan. Warna, spacing, tipografi, radius dan shadow diekstrak menjadi tokens bersama, tanpa tema baru atau framework frontend.
- Komponen — sembilan halaman memakai fondasi form/card/grid/table/list, alert/status/loading/empty/error, toast, fokus dan navigasi bersama. Mobile menu mendukung aria-expanded/Escape; skip link dan tabel geser dapat diakses keyboard. Kontrol terlihat minimum 44px dan animasi CSS mengikuti reduced-motion.
- Halaman — login memakai toggle kata sandi berupa button keyboard-accessible; antrean menampilkan teks status Indonesia dan feedback retry; dashboard awal memakai API laporan existing dengan guard owner/admin, tanpa data dummy dalam aplikasi. Halaman pelanggan/layanan/staf/shift/audit/tracking mengikuti tema yang sama; print shift tetap mempunyai override khusus.
- Aset — CSS/JavaScript inline login/antrean/staf dipisah, HTML dirapikan dan slang dihapus. URL/ID form/payload transaksi tetap; root frontend dan STATIC_DIR dipertahankan. Go/API/config/migration tidak diubah; embedding backend belum diaktifkan.
- Dokumentasi — komponen/tokens, kontras, batas dashboard dan cara verifikasi dicatat di docs/ui.md; README/arsitektur/backlog diperbarui. Tahap berikutnya Hari 19 — UI autentikasi/sesi dan pengelolaan staf.

## Verifikasi

- Edge headless dengan API mock lokal — sembilan halaman × lima lebar 1280/768/360/390/412px lulus: tanpa overflow halaman, satu landmark main, label form, target kontrol terlihat minimum 44px dan aset lokal tersedia. Screenshot desktop/mobile dibanding baseline; login, antrean, dashboard, layanan, menu ponsel, shift dan audit ditinjau visual.
- Focused browser recheck — menu/Escape/fokus, skip link/Tab, toggle kata sandi dengan Enter, login gagal/retry, queue/dashboard kosong, error/retry, loading, toast/tutup, offline nyata melalui emulasi jaringan, role guard kasir dan logout CSRF lulus. Hari dengan hanya refund tetap menampilkan refund/net negatif tanpa label kosong. Print media menyembunyikan header/sidebar, reduced-motion menonaktifkan spinner; tidak ada exception JavaScript/console.error atau error aset pada alur normal. Respons API 401/503 dan offline sengaja disimulasikan untuk failure state.
- `node --test scripts/frontend-contract.test.cjs` — 14 test lulus: pesan/login cookie, antrean/logout, staff permission/form, customer/history/tracking dan katalog/perhitungan input. Fixture diarahkan ke login.js/queue.js setelah ekstraksi; tidak menjalankan full backend suite.
- Syntax/asset check — sembilan HTML dan sepuluh file JS lulus `node --check`, UTF-8, referensi lokal, ID statis unik, CSS variable terdefinisi dan scope header tabel. Tidak ada blok CSS/JavaScript inline tersisa.
- Perhitungan kontras sRGB — teks/putih 12,63:1, muted/putih 7,56:1, action/putih 5,22:1, login/putih 5,17:1 dan border input/putih 3,54:1. Biru utama 3,98:1 digunakan sebagai aksen; bukan audit aksesibilitas menyeluruh.
- Tidak menjalankan tes backend sesuai cakupan. Tidak mengakses `.env`, PostgreSQL atau data operasional; tidak ada commit/push/deployment.
- UTF-8/tautan dokumentasi, referensi aset, diff dan status Git diperiksa; `git diff --check` bersih. Browser/server mock lokal dihentikan setelah verifikasi; harness/screenshot sementara berada pada cache di luar repository.
- Chime — chime-1.mp3 diputar dua kali melalui MediaPlayer .NET; masing-masing mencapai posisi akhir 2,376 detik.

## Perubahan Penting

- `frontend/ui/tokens.css`, `ui.css`, `ui.js` dan favicon SVG — sumber warna/komponen/shell bersama, safe DOM, navigasi sesuai role dan feedback koneksi.
- `frontend/login.html/js`, `index.html`/`queue.js`, `dashboard.html/js` — login/antrean terpisah dari HTML dan fondasi dashboard berdasarkan kontrak API existing.
- HTML/CSS/JS pelanggan/layanan/staf/shift/audit/tracking — komponen bersama, markup semantik, label/header tabel, empty list li dan hook presentasi sesi tanpa fetch session tambahan. Handler transaksi/idempotency tetap.
- `scripts/frontend-contract.test.cjs` — sumber script eksternal dan mock DOM mengikuti atribut baru; assertion pesan diperbarui ke bahasa produksi yang jelas.
- [ui.md](../ui.md), README, architecture dan backlog — tokens, penggunaan komponen, struktur aset, evidence dan batas cakupan Hari 18.

## Risiko atau Sisa Pekerjaan

- Verifikasi memakai browser desktop dan emulasi viewport/touch Android dengan data mock; Android/printer fisik serta alur UI–database nyata belum diverifikasi pada tugas ini.
- Dashboard masih ringkasan awal; filter/detail/export/cetak laporan lengkap tetap Hari 22. Acceptance sesi/staf/POS/antrean/receipt dan gate produksi sebelumnya tetap terbuka.
- Embedding Go belum dipasang; deployment harus menyertakan seluruh folder frontend termasuk ui/. Tidak ada blocker implementasi Hari 18 yang diketahui.
