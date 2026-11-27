# PWA dasar — tindakan dan hasil

Status Hari 23, 27 November 2026: PWA dasar tersedia pada frontend existing, tanpa perubahan Go/API/schema, CDN atau dependency frontend baru. Ini belum merupakan rilis produksi.

## Kondisi jaringan — batas penggunaan

- Membaca konteks proyek dan UI existing — target operasional laptop/Android, lokal Windows 11 dengan PostgreSQL lokal/VM Ubuntu; produksi direncanakan melalui HTTPS, reverse proxy dan Cloudflare Tunnel. Tidak ada bukti topologi LAN/Wi-Fi, koneksi cadangan atau akses langsung server ketika internet terputus; konfigurasi jaringan operasional tidak diubah/diklaim terverifikasi.
- Memisahkan koneksi perangkat dari ketersediaan layanan — `navigator.onLine` hanya petunjuk perangkat. Internet/Tunnel/server/database dapat gagal walaupun indikator online. Navigasi menampilkan shell umum saat fetch gagal, melewati batas 8 detik, atau menerima 502/503/504. Respons 401/403/404 dipertahankan, dan kegagalan API tetap ditangani UI existing.
- Membatasi offline pada penjelasan dan navigasi pemulihan — pemasangan pertama harus online hingga service worker aktif dan sembilan aset lengkap. Halaman bisnis tidak tersedia offline; shell tidak menjalankan modul sesi atau mengambil data bisnis. Tombol **Coba kembali** memuat URL halaman saat ini, tanpa menyalin query/token ke cache. Tidak otomatis membuka halaman setelah online.
- Mempertahankan transaksi existing — tidak ada Background Sync, IndexedDB transaksi, queue atau retry tulis otomatis. Online hanya memperbarui indikator dan memeriksa service worker. Pengguna memeriksa hasil transaksi dan memilih retry manual melalui workflow existing; draft/key masih hanya dalam memori tab. Jika tab terlanjur ditutup/reload, periksa riwayat sebelum memasukkan ulang.

## Manifest dan pemasangan — hasil

- Menambahkan `frontend/manifest.webmanifest` — identitas stabil `/`, nama ULaundry, bahasa Indonesia, start `/login.html`, scope `/`, display standalone dan warna existing `#007bff`/`#f4f4f9` tersedia. Start URL tidak mengandung identitas/token.
- Menautkan manifest, theme color, touch icon dan modul PWA pada sepuluh HTML existing — akses normal tetap mengikuti autentikasi/izin server. Offline shell juga menautkan manifest.
- Mengadaptasi bentuk U dari favicon lokal menjadi PNG 192×192 dan 512×512 — placeholder buatan proyek, tanpa aset merek pihak ketiga, font atau unduhan eksternal. Ikon `purpose: any`, tidak mengklaim maskable; ikon final dapat menggantikannya dengan versi cache baru.
- Menyediakan panel **Aplikasi ULaundry** — tombol **Pasang aplikasi** muncul ketika browser mengirim `beforeinstallprompt`. Setelah dipakai/dipasang, prompt disembunyikan. Browser lain dapat memakai menu pemasangan/Add to Home Screen; tombol/fitur bergantung dukungan browser. Tidak meminta izin push notification; pemberitahuan hanya di halaman.
- Memakai secure context — HTTPS diperlukan pada penggunaan operasional; localhost/loopback dapat dipakai untuk verifikasi. HTTP alamat LAN biasa bukan jalur PWA yang didukung. Pemasangan native OS/Android fisik belum dibuktikan.

## Cache — batas eksplisit

| Tindakan | Hasil |
| --- | --- |
| Precache saat install ke `ulaundry-shell-<VERSION>` | Tepat sembilan berkas publik: `/offline.html`, `/ui/tokens.css`, `/ui/ui.css`, `/pwa/pwa.css`, `/pwa/pwa.js`, `/manifest.webmanifest`, `/ui/favicon.svg`, `/icons/icon-192.png`, `/icons/icon-512.png` |
| Memvalidasi status dan Content-Type; fetch install tanpa cookie, tanpa HTTP cache, redirect ditolak | HTML login/error tidak dapat menggantikan CSS/JS/ikon. Seluruh berkas wajib berhasil; kegagalan menggagalkan worker baru dan menghapus cache parsial, worker lama tetap aktif |
| Mengutamakan jaringan dengan `cache: no-store` untuk navigasi HTML yang dikenal | Halaman terbaru dibaca online, bukan HTML deployment lama. Hanya saat outage shell umum dipakai; HTML bisnis tidak pernah ditulis ke Cache Storage |
| Mengutamakan jaringan untuk sembilan aset publik; cache install hanya sebagai fallback | Tidak ada runtime cache/write, stale-while-revalidate atau campuran aset baru yang disimpan ke cache versi lama. Respons jaringan/fallback diberi `Cache-Control: no-store` agar renderer tidak memakai memory cache lama |
| Melewatkan HTTP cache untuk script/CSS bisnis publik pada `NETWORK_ASSETS` | Fetch `no-store`, respons `no-store`, tanpa precache/fallback/runtime cache. JS deployment baru terbaca sekalipun sebelumnya ada respons HTTP dengan max-age panjang; daftar ini wajib diperbarui saat menambah entry point publik |
| Tidak memanggil `respondWith` di luar allowlist | Seluruh `/api/*` termasuk login/logout/session, customers, orders/payment/refund, struk, laporan/CSV, audit dan tracking tetap jaringan langsung; cookie/CSRF/izin/no-store server existing tetap berlaku |
| Melewatkan request non-GET, Authorization, Range, origin lain dan query aset | Tidak ada cache POST/PUT/DELETE, Bearer, data parsial, URL bertoken atau aset pihak ketiga. Query navigasi tidak disimpan sebagai key; hanya fallback shell konstan |
| Menghapus hanya cache berawalan `ulaundry-shell-` lama saat aktivasi | Cache aplikasi lain pada origin yang sama tidak dihapus. Tidak ada penghapusan cookie/sesi/storage pengguna |

Halaman bisnis statis (`index`, POS, dashboard, customers, services, staff, shifts, audit, tracking, login) dan script bisnis tidak diprecache. URL struk/API/URL tidak dikenal tidak mendapat fallback shell, termasuk saat dinavigasikan langsung. Data yang sudah tampil dan draft bisa tetap ada di memori tab offline; PWA tidak mempersistensikannya atau menyatakan transaksi sudah tersimpan. Unduhan/cetakan yang diminta pengguna tetap tanggung jawab pengelolaan perangkat, bukan cache PWA.

## Pembaruan dan release — hasil yang wajib dipertahankan

- Mendaftarkan `/sw.js` pada scope `/` dengan `updateViaCache: 'none'` — pemeriksaan worker melewati HTTP cache browser. Pemeriksaan dilakukan saat bootstrap, kembali online, tab terlihat, setiap lima menit selama terlihat, dan tombol **Periksa pembaruan**. Kegagalan ditampilkan tanpa menghalangi retry manual.
- Menahan worker baru dalam status waiting — panel membuka pesan **Versi baru tersedia**. Tidak ada `skipWaiting` saat install; pengguna memilih **Aktifkan versi baru** setelah memeriksa transaksi. Aktivasi menghapus cache versi lama dan mengambil kontrol tab; konten/draft tab tetap terbuka.
- Menampilkan **Muat ulang halaman** setelah controller berubah — berlaku pada tab pemicu dan tab lain, tanpa reload otomatis. Tombol menggunakan reload normal sehingga `beforeunload` existing tetap berlaku; pesan menjelaskan kehilangan draft/key. Aktivasi tidak berarti transaksi selesai. Pengguna harus menyelesaikan/mengecek hasil sendiri sebelum reload.
- Menetapkan `VERSION = '2026-11-27-1'` di `frontend/sw.js` — **setiap rilis frontend wajib mengganti versi**, termasuk perubahan HTML/CSS/JS/ikon dan rollback. Gunakan ID unik, misalnya tanggal + nomor/build SHA; jangan gunakan ulang versi yang pernah diaktifkan. Jika versi tidak berubah, perubahan aset saja tidak menghasilkan notifikasi worker baru.
- Menyiapkan deployment satu paket — kirim seluruh `STATIC_DIR` termasuk `sw.js`, manifest, `offline.html`, folder `pwa/`, `icons/` dan `ui/`, dengan MIME benar. Publikasikan secara atomik (direktori release/symlink) agar install tidak mencampur berkas; cache versioning bukan pengganti deployment atomik. Jangan rename URL worker pada tiap rilis.
- Menetapkan syarat reverse proxy/Cloudflare untuk deployment — bypass edge cache untuk `/api/*`, `/sw.js`, manifest, HTML dan aset tanpa hash; jangan gunakan Cache Everything. Beri `/sw.js`, manifest, HTML/CSS/JS tanpa hash `Cache-Control: no-cache` atau `no-store`; API privat wajib `no-store`. `updateViaCache` tidak membypass cache CDN yang salah konfigurasi. Go FileServer saat ini tetap digunakan tanpa perubahan; aturan proxy/Cloudflare belum diterapkan/diverifikasi pada tugas ini.
- Memverifikasi release sebelum produksi — worker URL harus memberi 200 JavaScript tanpa redirect/login/challenge, manifest valid dan semua ikon tersedia melalui HTTPS. Periksa deployment baru pada profil yang sebelumnya memakai versi lama, bukan hanya profil browser bersih. Pengguna online mendapat HTML baru walaupun worker masih waiting karena navigasi mengutamakan jaringan.

## Recovery — tindakan dan hasil

1. Selesaikan/periksa transaksi yang belum pasti sebelum menutup tab — key/draft memori tidak dipulihkan dari cache. Periksa pembaruan, aktifkan, lalu muat ulang secara manual.
2. Bila update gagal disiapkan, pulihkan aset/MIME/jaringan dan pilih **Periksa pembaruan** — cache parsial dibuang, versi sebelumnya tetap berfungsi. Bila server sehat tetapi shell muncul, pilih **Coba kembali**; pastikan proxy tidak melayani HTML shell lama dari cache.
3. Bila worker/cache rusak, buka DevTools → Application pada origin ULaundry yang benar — unregister hanya registration `/sw.js`, lalu hapus hanya Cache Storage `ulaundry-shell-*`. Muat ulang online untuk install ulang. Jangan memakai Clear site data menyeluruh karena dapat menghapus sesi dan data aplikasi lain. Unregister baru efektif penuh setelah tab lama ditutup; lakukan sesudah transaksi dipastikan.
4. Bila perlu rollback, kembalikan satu paket aset yang kompatibel dan beri `VERSION` baru — browser menerima rollback sebagai update baru; verifikasi waiting/aktivasi/reload, shell dan API kembali. Jangan menyalin worker lama dengan versi identik atau menghapus `/sw.js` menjadi 404 karena worker aktif bisa tetap bertahan.
5. Bila perlu menonaktifkan PWA darurat, siapkan worker pengganti pada URL yang sama dengan isi baru: aktivasi membersihkan hanya prefix milik ULaundry dan fetch langsung jaringan — review/deploy sebagai release khusus. Penghapusan file worker saja bukan recovery terjamin; tidak ada kill switch publik yang dijalankan pada tugas ini.

## Verifikasi terarah — bukti dan batas

- Menjalankan `node scripts/pwa-browser-check.cjs` — memakai Edge/Chromium headless pada server fixture loopback, API sintetis no-store dan profil sementara. Node 22+ dengan native WebSocket diperlukan; Windows memakai Edge default, host lain mengisi `PWA_BROWSER` ke executable Chromium yang sudah terpasang. Script tidak memasang dependency, membaca `.env` atau mengakses database. Browser dijalankan tanpa sandbox hanya untuk fixture lokal yang dipercaya; bukan konfigurasi browser operasional.
- Memeriksa manifest/installability — `Page.getAppManifest` dan `Page.getInstallabilityErrors` tanpa error, dimensi/header PNG valid, registration scope `/` dan `updateViaCache: none` terbukti. Event prompt sintetis hanya membuktikan wiring tombol, bukan pemasangan OS.
- Memutus jaringan dan navigasi ulang — enam URL bisnis/publik mendapat shell tanpa PII; GET API sensitif dan POST gagal jaringan, bukan cache. Cache tetap tepat sembilan key tanpa query. Authorization/query aset tidak mendapat fallback; respons fallback no-store.
- Memulihkan online secara manual dan mengganti deployment — tidak ada replay POST atau navigasi otomatis; retry kembali ke halaman online. HTML baru dan script fixture baru langsung terbaca meskipun script sebelumnya memiliki HTTP max-age satu tahun. Respons 503/timeout menuju shell dan 403 tetap dipertahankan.
- Mengganti worker A→B dengan dua tab — notifikasi waiting tampil, aktivasi eksplisit mempertahankan draft dua tab, cache lama dibersihkan dan reload hanya setelah klik. Install C gagal karena ikon 503 mempertahankan B; retry C dan rollback dengan versi unik lulus. Recovery unregister/hapus cache prefix lalu registrasi ulang diperiksa.
- Memeriksa lebar 1280/360/390/412px serta print — panel tanpa overflow halaman dan disembunyikan saat cetak. Tidak ada exception JavaScript halaman/console.error. Error jaringan/503 saat skenario kegagalan memang disengaja.
- Membatasi klaim — bukti ini menguji frontend dan lifecycle browser dengan fixture, bukan cookie Secure lewat HTTPS produksi, reverse proxy/Tunnel nyata, integrasi PostgreSQL, pending financial intent nyata, Android fisik atau native OS install. Tidak menjalankan backend test karena Go/API tidak berubah.

Referensi platform: [MDN — service worker lifecycle](https://developer.mozilla.org/en-US/docs/Web/API/Service_Worker_API/Using_Service_Workers), [updateViaCache](https://developer.mozilla.org/en-US/docs/Web/API/ServiceWorkerRegistration/updateViaCache), [installability](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/Guides/Making_PWAs_installable). Bukti tugas: [laporan Hari 23](progress/day-23.md).
