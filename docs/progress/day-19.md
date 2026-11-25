# Laporan Hari 19 — UI Autentikasi, Sesi dan Pengelolaan Staf

## Tindakan — Hasil

- Login — terhubung ke cookie HttpOnly Hari 09 dan memastikan GET session cocok sebelum redirect; CSRF/identitas hanya di memori, tanpa penyimpanan credential browser. Password dibersihkan setelah submit dan pesan gagal generik tidak membocorkan detail backend. Retry-After membatasi tombol tanpa percobaan masuk otomatis.
- Sesi — tujuh halaman terlindungi memakai transport same-origin/CSRF bersama, pemeriksaan terdeduplikasi, timer expiry serta pemeriksaan focus/visibility/BFCache. Data disembunyikan selama pemeriksaan/gangguan dan saat sesi berakhir; 401 mengarahkan sekali dengan replace ke login publik yang tidak melakukan redirect otomatis. Respons terlambat atau perubahan identitas tidak menghidupkan akses lama.
- Logout — cookie/CSRF dan konfirmasi server menentukan hasil; kegagalan menyediakan retry, mutasi yang masih berjalan mengunci tombol keluar. Logout antar-tab menyampaikan event tanpa token/identitas; sesi target tetap dicabut oleh backend existing.
- Owner — daftar 20 akun per halaman, create, ubah role, deactivate/reactivate dan reset password memakai API/version existing. Kasir/admin tidak meminta daftar staf atau melihat kontrol pengelolaan. Validasi per kolom/fokus, konfirmasi, pembersihan password dan konflik/hasil tidak pasti mewajibkan reload; tidak ada retry mutasi otomatis oleh UI.
- Tampilan/dokumentasi — palet/tokens dan layout responsif Hari 18 dipertahankan, tanpa framework/dependency baru. Panduan UI, README, arsitektur dan backlog diperbarui; Go, endpoint serta migration tidak berubah, schema tetap 12. Tahap berikutnya Hari 20 — UI POS/pembuatan order.

## Verifikasi

- `node --test scripts/frontend-contract.test.cjs` — 22 test lulus, termasuk modul sesi nyata dengan transport sintetis: deduplikasi, CSRF, tujuan lintas origin/Bearer ditolak, mutasi tidak diulang, 401 tunggal, outage/pemulihan, identitas berubah, focus/logout saat mutasi dan respons terlambat. Fixture halaman mengikuti transport bersama; customer/catalog/tracking tetap lulus.
- Edge headless dengan API mock lokal — login sukses, akun/password salah dengan pesan identik, 429/cooldown, 500, cookie tidak diterima dan tanpa redirect loop lulus. Cookie HttpOnly tidak terbaca JavaScript dan storage browser tidak berisi token/credential.
- Browser sesi/izin — expiry timer, revocation saat pemulihan BFCache, pemeriksaan awal/lanjutan 503 lalu retry, forbidden admin/kasir, submit paksa kasir ditolak, logout gagal/retry/CSRF, logout antar-tab dan reset diri sendiri kembali ke login lulus. Tidak ada exception JavaScript atau console.error; HTTP gagal sengaja disimulasikan.
- Browser owner — create/double-submit, perubahan role, deactivate/reactivate, reset target, pagination limit/offset, pembersihan password, UTF-8 melebihi 72 byte, konfirmasi tidak cocok, stale version, respons sukses tidak terbaca dan refresh gagal setelah create terkonfirmasi lulus. Nama dengan markup tampil sebagai teks aman.
- Desktop/Android viewport — tujuh halaman × 1280/360px lulus bootstrap satu GET session per navigasi dan tanpa overflow halaman; login/staf juga diperiksa pada 390px. Screenshot desktop/form mobile, skip link/Tab, Enter memilih akun, fokus editor, menu/Escape ditinjau.
- Syntax/aset — sembilan HTML dan sebelas file JavaScript lulus pemeriksaan syntax, UTF-8, referensi lokal, ID unik, CSS tokens dan scope header tabel. Diff/status serta tautan dokumentasi diperiksa; `git diff --check` bersih.
- Cakupan pengujian — tidak menjalankan test backend HTTP/auth atau full database suite karena endpoint tidak berubah; tidak mengakses `.env`, PostgreSQL atau data operasional. Harness/screenshot sementara berada di cache luar repository; browser/server mock dihentikan setelah verifikasi. Tidak ada commit/push/deployment.
- Chime — chime-1.mp3 diputar dua kali melalui MediaPlayer .NET; masing-masing mencapai posisi akhir 2,376 detik.

## Perubahan Penting

- `frontend/ui/session.js`, `ui.js` dan wiring tujuh halaman — satu pengelola sesi/transport/logout dengan pembatasan data terlindungi dan navigasi sesuai role.
- `frontend/login.html/js` — alasan sesi berakhir, konfirmasi cookie, pesan aman, cooldown dan penghapusan password.
- `frontend/staff.html/js/css` — validasi aksesibel, operasi akun sesuai permission, pagination, proteksi submit ganda dan penanganan hasil tidak pasti.
- `scripts/frontend-contract.test.cjs` — 22 test kontrak halaman dan keamanan transport sesi.
- [ui.md](../ui.md), README, architecture dan backlog — kontrak komponen, tindakan — hasil, bukti dan batas verifikasi Hari 19.

## Risiko atau Sisa Pekerjaan

- Browser memakai API mock HTTP lokal; cookie Secure/HTTPS produksi, Android fisik dan alur UI–database nyata belum diverifikasi pada Hari 19. Pengujian ini tidak menggantikan acceptance backend/operasional sebelum rilis.
- POS/order, antrean/receipt dan laporan lengkap tetap mengikuti Hari 20–22. Deployment masih perlu menyertakan seluruh frontend/ui; embedding Go belum diaktifkan. Tidak ada blocker implementasi Hari 19 yang diketahui.
