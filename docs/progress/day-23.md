# Laporan Hari 23 — PWA Dasar dan Pemulihan Koneksi

## Tindakan — Hasil

- Manifest/installability — sepuluh halaman existing dan offline shell memakai manifest lokal, identitas ULaundry, start login, scope root dan tema biru existing. Ikon PNG 192/512 sah diadaptasi dari favicon proyek tanpa aset pihak ketiga; panel pemasangan mengikuti dukungan browser.
- Offline shell — navigasi mengutamakan jaringan, dengan shell tanpa data bisnis saat fetch gagal, timeout 8 detik atau 502/503/504. Status offline dan tombol Coba kembali tersedia; pulih online tidak memaksa navigasi atau mengirim transaksi ulang.
- Privasi cache — Cache Storage dibatasi sembilan berkas publik saat install, tanpa runtime cache/API/auth/customer/order/receipt/report/audit/tracking. Request tulis, Authorization, Range, query aset dan origin lain dilewatkan; tidak ada queue offline atau Background Sync.
- Update — worker diperiksa tanpa HTTP cache browser, saat bootstrap/online/tab terlihat, berkala dan manual. Notifikasi waiting, aktivasi eksplisit, penghapusan cache lama dan reload manual tersedia. Tab/draft tetap terbuka saat aktivasi; pesan mengingatkan key retry hanya dalam memori.
- Recovery/dokumentasi — versi unik setiap rilis/rollback, install gagal, scoped unregister/cache cleanup, deployment atomik serta syarat bypass cache proxy/Cloudflare didokumentasikan. Tidak ada perubahan Go/API/schema/dependency, akses database, commit, push atau deployment publik.

## Verifikasi

- `node scripts/pwa-browser-check.cjs` — lulus pada Edge headless/server fixture loopback. Manifest dan Chromium installability tanpa error, header/dimensi PNG valid, scope root dan updateViaCache none sesuai. Wiring prompt diuji dengan event sintetis, bukan pemasangan OS.
- Offline/privasi — enam navigasi bisnis/publik mendapat shell umum; GET sensitif dan POST gagal jaringan tanpa fallback data. Tepat sembilan cache key tanpa query; Authorization/query aset tidak mendapat cache dan fallback diberi no-store. Online tidak mereplay POST; retry manual kembali ke halaman jaringan.
- Freshness/outage — HTML dan JS deployment baru langsung terbaca, termasuk fixture JS dengan HTTP max-age satu tahun; request/respons aset dikontrol memakai no-store tanpa cache offline bisnis. Respons 503 dan navigasi melewati timeout menuju shell, 403 tetap dipertahankan. Worker A→B memberi notifikasi pada dua tab, mempertahankan draft saat aktivasi dan menunggu klik reload; cache lama dibersihkan.
- Kegagalan/recovery — ikon 503 menggagalkan install C tanpa menghapus versi B; retry C, rollback ber-ID baru dan unregister/hapus cache prefix lalu registrasi ulang lulus.
- Visual — screenshot offline/update/desktop/ponsel ditinjau; panel mengikuti alur halaman dan tidak menutupi form. Lebar 1280/360/390/412px tanpa overflow, panel tersembunyi saat print; tidak ada exception JavaScript halaman atau console.error.
- Syntax/aset/diff — JavaScript baru diperiksa dengan `node --check`; metadata/aset sebelas HTML, dokumen/tautan serta Git diff/status diperiksa. Tidak menjalankan backend test karena Go/API tidak berubah.
- Chime — diputar dua kali melalui MediaPlayer, masing-masing berdurasi 2,376 detik.

## Perubahan Penting

- `frontend/manifest.webmanifest`, `icons/`, `offline.html`, `sw.js` — instalasi, aset publik terbatas, network-first, shell dan lifecycle versi.
- `frontend/pwa/pwa.js` dan `pwa.css`, sepuluh HTML existing — indikator koneksi, pemasangan, notifikasi update dan reload manual tanpa overlay form.
- `scripts/pwa-browser-check.cjs` — verifikasi PWA mandiri dengan fixture lokal, profil sementara dan teardown browser/server.
- [pwa.md](../pwa.md), README, UI, arsitektur dan backlog — panduan jaringan/cache/release/recovery, command verifikasi dan status Hari 23.

## Risiko atau Sisa Pekerjaan

- HTTPS produksi, reverse proxy/Cloudflare Tunnel/cache rules, pemasangan native OS, Android fisik dan acceptance browser–database belum diverifikasi. Tidak ada bukti topologi LAN/Wi-Fi atau akses lokal ketika internet terputus.
- Setiap release frontend wajib mengganti versi worker dan menerapkan aturan cache proxy yang terdokumentasi. Instalasi pertama membutuhkan online; PWA tidak membuat operasi bisnis tersedia offline.
- Draft/key transaksi tetap dalam memori tab. Pengujian dua tab memakai draft formulir sintetis; pending financial intent nyata tidak diuji ulang. Periksa hasil transaksi sebelum reload/menutup tab.
- Tidak ada blocker implementasi Hari 23 yang diketahui; kesiapan produksi tidak diklaim.
