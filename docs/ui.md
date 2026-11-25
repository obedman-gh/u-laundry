# UI ULaundry — Hari 18–21

Fondasi bersama memakai HTML semantik, CSS dan JavaScript tanpa framework/build frontend. Identitas ULaundry/Perumahan Viola dan palet existing dipertahankan. Sepuluh halaman memakai tokens bersama: login, antrean, POS, dashboard, pelanggan, layanan, staf, kas/shift, audit dan tracking. Hari 21 menambah search API, saldo/proyeksi detail dan receipt thermal; konfigurasi dan migration tetap, schema 12.

## Tokens dan asal warna

Sumber tunggal stylesheet: `frontend/ui/tokens.css`. Nilai berasal dari CSS antrean/login/customers/shifts/audit sebelumnya, bukan palet baru.

| Token | Nilai | Pemakaian dan sumber lama |
|---|---|---|
| `--color-brand` | #007bff | Aksen, garis aktif, spinner; antrean dan halaman operasional |
| `--color-link`, `--color-action` | #0069d9; action alias link | Tombol utama aplikasi; warna link/staf dan tombol shift existing |
| `--color-action-hover` | #0056b3 | Hover/fokus/link navigasi aktif; customers/shifts/audit |
| `--color-login` / `--color-login-hover` | #2563eb / #1d4ed8 | Tombol login existing, tetap terpisah dari aksen aplikasi |
| `--color-background` / `--color-background-alt` | #f4f4f9 / #f3f4f6 | Latar aplikasi / login dan permukaan sekunder |
| `--color-surface` | #fff | Card, form, navigasi |
| `--color-text` / `--color-heading` | #333 / #1f2937 | Teks aplikasi / heading dari login existing |
| `--color-muted` | #4b5563 | Teks penjelasan, dari login |
| `--color-border` / `--color-input-border` | #ddd / #888 | Pembatas tabel/card dan batas input dari audit existing |
| `--color-danger` | #dc2626 | Penanda error, dari login |

Teks putih pada action #0069d9 mempunyai kontras 5,22:1, tombol login 5,17:1, teks #333 pada putih 12,63:1, muted 7,56:1 dan border input #888 pada putih 3,54:1. #007bff pada putih 3,98:1 dipakai sebagai aksen, bukan teks kecil tombol utama. Nilai dihitung dari luminance sRGB; ini pemeriksaan pasangan dasar, bukan sertifikasi aksesibilitas menyeluruh. SVG favicon U memakai #007bff/putih yang sama; wordmark ULaundry tetap tampil sebagai teks.

| Kelompok | Tokens |
|---|---|
| Tipografi | `--font-body` Arial/sans-serif; `--font-login` Segoe UI/Tahoma; `--font-mono` Consolas; ukuran small .875rem, base 1rem, section 1.25rem, title clamp 1.5–1.875rem |
| Spacing | `--space-1/2/3/4/5/6/8/12` = .25/.5/.75/1/1.25/1.5/2/3rem |
| Bentuk | `--radius-small` 4px, `--radius-card` 8px; shadow card 0 2px 4px dan login 0 4px 6px dengan hitam .1 seperti existing |
| Layout | control 44px, sidebar 14rem, content maksimal 1280px; breakpoint navigasi 767px |

Gunakan `var(--token)` pada CSS baru. Warna dan geometri dasar tidak didefinisikan ulang dalam CSS halaman. Override halaman dipakai untuk kebutuhan khusus, terutama tabel kas dan unit fisik cetak.

## Susunan aset

```text
frontend/
  ui/tokens.css       palette, typography, spacing, radius, shadows
  ui/ui.css          shell, forms, cards, tables/lists, states, responsive rules
  ui/ui.js           navigation, session presentation, feedback and safe DOM helpers
  ui/session.js      cookie/CSRF transport, expiry, logout and protected content gate
  ui/favicon.svg     favicon lokal dalam palet existing
  login.html/js      login dan toggle kata sandi
  index.html         antrean, URL / tetap
  queue.js/css       antrean, detail, status, pembayaran/refund dan lifecycle command
  queue-model.js     validasi snapshot, izin, nominal dan keterlambatan
  receipt.js/css     kontrol cetak dan lebar fisik struk privat dari handler Go
  pos.html/js/css    alur pembuatan pesanan dan hasil/tautan struk
  pos-model.js       validasi, konversi gram dan perkiraan katalog integer
  dashboard.html/js  ringkasan awal owner/admin
  customers.*        pelanggan dan histori
  services.*         katalog
  staff.*            pengelolaan akun
  shifts.*           kas/rekonsiliasi dan print overrides
  audit.*            penelusuran audit
  tracking.*         tracking publik
```

Urutan halaman: tokens.css → ui.css → CSS khusus halaman, lalu ui.js → ui/session.js → JS halaman, semuanya lokal dan script `defer`. Tracking publik hanya memakai ui.js dan tracking.js. HTML tidak mempunyai blok CSS/JavaScript inline. URL, enum, payload dan aturan transaksi tetap; fixture frontend mengikuti script halaman terpisah.

Server tetap melayani `STATIC_DIR` melalui `http.Dir`; salin seluruh `frontend/`, termasuk `ui/`, sebagai satu set. Aset tidak membutuhkan npm, CDN, font eksternal atau proses build frontend. Folder ini dapat dibungkus `embed.FS` pada pekerjaan packaging berikutnya; embedding Go belum diaktifkan agar konfigurasi/lifecycle backend tidak ikut berubah pada tugas UI ini.

## Kerangka dan komponen

| Komponen | Kontrak |
|---|---|
| Shell staf | `<body data-staff-app data-page="orders">` dengan satu `<main>`. ui.js menambahkan header/wordmark, navigasi utama, identitas sesi, tombol keluar dan skip link. Page keys: orders/pos/dashboard/customers/services/staff/shifts/audit |
| Sesi/navigasi | `ULaundrySession.get()` mendeduplikasi pemeriksaan GET session dan menyerahkan identitas ke shell. Data halaman disembunyikan sampai sesi terkonfirmasi; link mengikuti role. Izin API tetap ditegakkan server |
| Navigasi ponsel | Tombol Menu dengan aria-expanded/controls; disclosure dalam alur dokumen. Escape mengembalikan fokus ke tombol. Sidebar muncul pada desktop, menu dua kolom pada lebar ≤767px; tanpa focus trap modal |
| Heading/action | `.page-heading`, `.toolbar`, `.pager`/`#pager`; kontrol membungkus pada layar kecil |
| Form | Label for/id atau label pembungkus, input/select/textarea dengan tinggi minimum 44px, focus-visible 3px. Validasi native, atribut existing dan validasi harga integer tetap |
| Card/grid | `.card`, `.grid`, `.fields`, `.summary-grid`; minmax menyesuaikan satu/dua/beberapa kolom tanpa memperlebar halaman |
| Table/list | `<table>` dengan th scope col; `.scroll` diberi region, label dan tabindex untuk akses keyboard. Scroll horizontal hanya dalam tabel. Daftar memakai ul/li termasuk hasil kosong |
| Badge/status | `.status`/`.badge`; status antrean ditampilkan dalam bahasa Indonesia dan tetap berbasis enum server |
| Alert | `.ui-alert` dan data-state info/success/loading/error/offline. Pesan tetap terlihat; error memakai role alert, lainnya status |
| Loading/empty/error | `ULaundryUI.state(target,{kind,title,description,retry})` membuat DOM semantik dan aria-busy. Queue/dashboard memakai state lengkap dengan retry; halaman lama memakai message/loading yang sama |
| Toast | `ULaundryUI.notify(text,kind)`; dapat ditutup, sukses/info hilang sesudah tujuh detik, error tetap sampai ditutup. Pesan transaksi penting tetap memakai feedback persisten |
| Dialog | Konfirmasi perubahan staf/penutupan shift memakai dialog native browser existing; keyboard dan fokus dikelola browser |
| Offline | Banner koneksi dan feedback gagal; tidak ada pengiriman ulang transaksi otomatis/antrean offline. Saat online hanya pemberitahuan, pengguna memilih muat ulang |

Contoh pemakaian komponen:

```html
<link rel="stylesheet" href="/ui/tokens.css">
<link rel="stylesheet" href="/ui/ui.css">
<script src="/ui/ui.js" defer></script>
<script src="/ui/session.js" defer></script>
<script src="/halaman.js" defer></script>
<!-- Dalam body data-staff-app data-page yang sesuai -->
<main><section class="card"><h1>Judul halaman</h1></section></main>
```

```javascript
const session = await window.ULaundrySession.get();
const response = await window.ULaundrySession.fetch('/api/contoh'); // gunakan route API existing
window.ULaundryUI.message(document.getElementById('message'), 'Perubahan tersimpan.', 'success');
window.ULaundryUI.state(container, {
  kind: 'error', title: 'Data belum dapat dimuat',
  description: 'Periksa koneksi lalu coba kembali.', retry: load
});
```

Helper hanya membentuk elemen dan textContent, tanpa innerHTML atau template string HTML dari data. Sesi/CSRF hanya di memori seperti alur existing; tidak menambahkan penyimpanan JWT/token di localStorage. Login tetap membersihkan token legacy. Tracking tetap membaca fragmen token tanpa mengirimnya sebagai URL halaman atau mengubah projection publik.

## Batas halaman dashboard

`/dashboard.html` adalah fondasi ringkasan owner/admin: pesanan hari ini, penerimaan, piutang, shift aktif, status antrean dan net setelah refund. Data berasal dari `/api/laporan/dashboard` existing, tanpa nilai demo/fallback dalam source aplikasi. Sebelum data valid tidak ada angka sementara; kegagalan menyembunyikan angka lama dan menawarkan retry. Bilangan harus aman ditampilkan sebagai integer JavaScript.

Kasir yang membuka URL tersebut mendapat petunjuk menuju alur operasional tanpa request laporan sensitif. Halaman tidak melakukan polling; refresh membaca laporan dan mengikuti audit server existing. Filter periode, detail laporan, CSV serta penerimaan UI finansial lengkap tetap Hari 22. UI login/sesi/staf diverifikasi pada Hari 19 dan pembuatan order pada Hari 20; antrean/progres/receipt tersedia pada Hari 21. Verifikasi browser mock belum menutup AC15 atau kesiapan produksi seluruh aplikasi.

## Autentikasi dan pengelolaan akun — tindakan — hasil

| Tindakan | Hasil |
|---|---|
| Integrasi Hari 09 | POST `/api/login` mengirim JSON melalui same-origin, lalu GET `/api/session` memastikan cookie benar-benar diterima serta username/CSRF sesuai respons login sebelum redirect. JWT tetap hanya cookie HttpOnly; CSRF/identitas/expiry hanya dalam memori. Token legacy dibersihkan pada login dan halaman staf. Tidak ada credential di localStorage/sessionStorage, query, log UI atau BroadcastChannel |
| Validasi login | Nama pengguna dan password wajib; password maksimal 72 byte UTF-8. Minimum password baru tidak diterapkan pada login agar akun existing tetap dapat masuk. 401 memberi satu pesan generik tanpa membedakan akun/password. 400/403/5xx memakai teks terkontrol; 429 mengikuti Retry-After angka, dibatasi 900 detik, tanpa mencoba masuk otomatis. Password dibersihkan segera setelah submit, gagal dan saat halaman disembunyikan |
| Sesi dan expiry | `get`, `recheck`, `fetch`, `logout`, `end`, `forbid` menjadi API modul bersama. Fetch hanya menerima route same-origin `/api/`, memakai cookie/no-store dan menolak Authorization. Mutasi memakai CSRF sesi aktif. Timer mengacu expires_at dan Date server; focus, visibility serta pemulihan BFCache memeriksa sesi kembali tanpa memperpanjang masa berlaku |
| Redirect tanpa loop | 401 pada halaman terlindungi menyembunyikan data, membersihkan input/list dan memakai location.replace sekali ke login dengan reason yang diizinkan. Respons sesi terlambat tidak menghidupkan sesi terminal. Login tetap publik, tanpa pemeriksaan/redirect otomatis saat dibuka; parameter next tidak diikuti. Perubahan actor/role/CSRF meminta masuk ulang |
| Gangguan pemeriksaan sesi | Offline/503 menyembunyikan konten dan menyediakan Coba kembali; bukan tanda logout. Pemulihan pemeriksaan awal membutuhkan reload agar bootstrap data lengkap. Permintaan tulis tidak diulang oleh modul; server tetap otoritas akses dan hasil transaksi |
| Logout | POST `/api/logout` memakai cookie/CSRF. Baru setelah sukses atau 401, UI menganggap sesi berakhir. Kegagalan menyediakan retry eksplisit. Keluar dinonaktifkan selama mutasi masih menunggu respons. Event signed-in/signed-out antar-tab tidak berisi identitas/token; tab terlindungi memeriksa perubahan login atau segera menyembunyikan data saat logout |
| Owner `/staff.html` | Daftar berpaginasi 20 akun, create, pilih akun, ubah role, deactivate/reactivate dan reset password memakai API existing serta expected version. Kasir/admin tidak meminta daftar staf, tidak melihat form atau navigasi pengelolaan; handler UI juga menolak submit tanpa hak. Permission server tetap wajib |
| Validasi staf | Label/aria-describedby, pesan per kolom, aria-invalid dan fokus kolom pertama; nama 1–100 karakter, username ASCII 3–50, enum peran eksplisit, password baru minimal 12 karakter/maksimal 72 byte sesuai policy Hari 09. Konfirmasi password wajib. Respons error backend tidak ditampilkan mentah; konfirmasi native tidak memuat password |
| Perubahan sensitif | Password dibersihkan setelah submit, gagal, pembatalan reset dan pergantian pilihan. Pemilihan akun memfokuskan heading editor dan menandai tombol aria-pressed. Role/status sendiri dinonaktifkan; reset sendiri meminta login ulang. Server menjaga owner aktif terakhir dan mencabut sesi target. Konflik/hasil tidak pasti menghapus pilihan dan mewajibkan muat ulang sebelum mutasi berikutnya; create terkonfirmasi dengan refresh gagal tetap dilaporkan berhasil tersimpan |
| Tema dan aset | Palet/tokens Hari 18 tetap. Tidak ada dependency, framework frontend, endpoint, Go atau migration baru; schema tetap 12. Semua tujuh halaman terlindungi memakai transport sesi bersama; tracking publik tetap terpisah |

## Verifikasi Hari 19 — tindakan — hasil

- `node --test scripts/frontend-contract.test.cjs` — 22 test kontrak/DOM sintetis, termasuk modul sesi nyata: deduplikasi, cookie/CSRF, penolakan lintas origin/Bearer, tanpa retry mutasi, 401 satu kali, outage/identitas berubah, race focus/logout saat mutasi dan respons terlambat. Mock transport halaman tidak menggantikan pengujian browser.
- Edge headless/API mock lokal — login sukses/gagal generik, 429, cookie tidak diterima, logout sukses/gagal/retry, logout antar-tab, expiry timer/BFCache, pemeriksaan sesi awal/lanjutan gagal lalu pulih, forbidden admin/kasir dan submit paksa ditolak; hasil dicatat di [laporan Hari 19](progress/day-19.md).
- Owner dengan data sintetis — create/double-submit, role, deactivate/reactivate, reset target/diri sendiri, pagination, UTF-8 password terlalu panjang, konfirmasi tidak cocok, stale version dan respons mutasi tidak terbaca diperiksa. Tidak ada pengiriman ulang oleh UI; staf wajib memeriksa daftar setelah hasil tidak pasti.
- Desktop 1280px dan viewport Android 360px — tujuh halaman terlindungi bootstrap dengan satu GET session per navigasi, tanpa overflow halaman; login/staf juga diperiksa pada 390px. Skip link/Tab, Enter untuk memilih akun, fokus editor, menu/Escape dan screenshot form ditinjau. Tidak ada exception JavaScript atau console.error; HTTP gagal sengaja disimulasikan untuk failure state.
- Backend tetap — tidak menjalankan test HTTP/auth backend atau full PostgreSQL suite karena endpoint tidak berubah. Browser memakai mock, bukan bukti cookie Secure melalui HTTPS produksi, Android fisik atau integrasi UI–database nyata. Harness/screenshot sementara berada di cache luar repository; ulangi skenario di atas pada lingkungan acceptance dengan data sintetis sebelum rilis.

## POS Hari 20 — tindakan — hasil

| Tindakan | Hasil |
|---|---|
| Masuk ke POS | `/pos.html` tersedia untuk owner/admin/cashier melalui navigasi dan tombol Pesanan baru pada antrean. Sesi/cookie/CSRF tetap memakai modul Hari 19; layout desktop dua kolom dan ponsel satu kolom memakai tokens/palet existing |
| Pilih pelanggan | Cari nama/kontak dengan pagination 20 hasil; hanya ID/nama/kontak yang dirender melalui createElement/textContent. Pelanggan tanpa kontak didukung. Enter untuk mencari, hasil pertama mendapat fokus, lalu pemilihan memfokuskan layanan |
| Buat pelanggan | Nama dan kontak opsional memakai API existing, tanpa alamat/profil tambahan. Submit ganda dikunci dan hasil sukses langsung dipilih. Karena POST customer belum idempotent, hasil tidak pasti menonaktifkan create hingga pencarian berhasil; pelanggan lama tidak tetap terpilih saat create baru dikirim |
| Pilih layanan | Daftar aktif dimuat per 100 layanan, dengan muat berikutnya dan refresh harga eksplisit. 1–100 baris dapat ditambah/dihapus; setiap baris memilih layanan, berat kg atau jumlah item, harga dan perkiraan subtotal. Katalog kosong/gagal dan layanan yang belum tersedia mempunyai feedback |
| Hitung perkiraan | Berat koma/titik maksimal tiga desimal diubah menjadi gram integer; item wajib integer positif. BigInt menghitung pembulatan half-up per baris dan penjumlahan tanpa floating point uang. Minimum 1 gram/1 item dan harga/total Rp0 dipertahankan. UI menolak angka di luar Number.MAX_SAFE_INTEGER agar tidak menampilkan rupiah yang sudah kehilangan presisi |
| Diskon/rincian | Nominal dan alasan hanya owner/admin; cashier tidak melihat field dan validator menolak diskon positif meski DOM dimanipulasi. Estimasi opsional ditafsirkan WITA secara eksplisit, bukan timezone perangkat; kosong memakai durasi katalog terlama di server. Catatan maksimal 1000 karakter |
| Total final | POST hanya mengirim customer_id, service_id, gram/jumlah, diskon/alasan, catatan dan override estimasi bila diisi. Harga/subtotal/total/status tidak dikirim. Tidak ada endpoint quote baru: sebelum submit angka berlabel perkiraan; setelah commit UI memakai snapshot harga/item/subtotal/diskon/total server, termasuk pemberitahuan bila berbeda dari perkiraan |
| Double-click/retry | Sebelum POST dibuat satu key acak kriptografis dan satu body JSON tetap. Saat mengirim, kontrol dikunci. Network/timeout 20 detik/5xx/respons tidak terbaca masuk hasil tidak pasti; hanya tombol pemeriksaan/retry eksplisit mengirim key dan body identik. Tidak ada replay saat online otomatis atau pembuatan key baru diam-diam. Replay 200 mengembalikan order semula, tanpa token tracking baru |
| Penolakan server | Penolakan 4xx pertama menyediakan Perbaiki pesanan, lalu intent baru memakai key baru. Sesudah hasil tidak pasti, 4xx berikutnya tetap mengunci payload karena penolakan saat ini tidak membuktikan hasil percobaan sebelumnya. Form tidak dibuka untuk membuat order pengganti secara otomatis |
| Offline/navigasi | Offline sebelum submit tidak mengirim atau mengantrekan order. Pending key/body dan data draft hanya di memori tab; navigasi tab yang sama dan klik Keluar diblokir selama hasil belum dipastikan, serta beforeunload memberi peringatan draft/pending. Redirect expiry tetap berjalan tanpa dialog tersebut. Reload/penutupan paksa kehilangan draft/key: periksa antrean sebelum memasukkan ulang, bukan menganggap pesanan gagal |
| Hasil/receipt | ID, item/kuantitas/harga snapshot, total final, estimasi dan status pembayaran dirender dari respons server. Tautan struk privat existing dibuka di tab baru; tracking menggunakan fragment hanya bila token tersedia pada respons pertama. Pesanan berikutnya membersihkan draft/hasil/tautan lama dan memfokuskan pencarian pelanggan. Pembayaran tetap terpisah; penyempurnaan antrean/receipt/cetak Hari 21 |

## Verifikasi POS Hari 20 — tindakan — hasil

- `node --test scripts/frontend-contract.test.cjs` — 29 test lulus; tujuh tambahan POS memeriksa konversi gram/presisi, pembulatan per baris, body tanpa harga/total klien, izin/batas diskon, estimasi WITA, respons snapshot server serta input tidak lengkap/terlalu banyak baris.
- Edge headless/API mock lokal — create campuran kg/item, double-submit satu POST/satu order, respons sukses tidak terbaca lalu replay key/body identik, perubahan harga server, penolakan 409, 503 lalu 403 yang tetap terkunci, offline tanpa antrean/auto-send, create customer ganda/hasil tidak pasti, owner discount/total nol dan override WITA lulus.
- Browser/aksesibilitas — pencarian customer tanpa kontak/markup aman, katalog kosong/gagal/retry, hapus baris, fokus hasil/kolom validasi, Enter/Tab dan viewport 1280/768/360/390/412px tanpa overflow diperiksa. Struk dibuka dengan API mock; tidak ada exception JavaScript atau console.error. Expiry pada submit mengarahkan ke login tanpa loop/dialog draft.
- Backend tetap — tidak menambah handler/use-case test atau menjalankan full suite karena Go/API/domain/transaction tidak berubah. Browser mock membuktikan perilaku klien, bukan konkurensi PostgreSQL atau cetak printer/Android fisik. Bukti dan batas ada pada [laporan Hari 20](progress/day-20.md).

## Antrean, detail dan transaksi Hari 21 — tindakan — hasil

| Tindakan | Hasil |
|---|---|
| Search/filter | `/index.html` memakai GET `/api/pesanan?search=...&status=...&limit=20&offset=0`. Search adalah substring literal ID/nama snapshot, tanpa peka kapital, trim dan maksimal 100 karakter; `%`/`_` bukan wildcard, nomor tidak dinormalisasi menjadi kontak. Search/status diproses bersama sebelum pagination di PostgreSQL. Filter kosong dihilangkan dari query; lima status operasional dipisahkan dari status pembayaran |
| Pagination | Terapkan filter/Enter kembali ke halaman 1; sebelumnya/berikutnya memakai 20 hasil dan urutan created_at/ID menurun. Tidak ada total palsu atau filter lokal pada satu halaman. Halaman penuh terakhir dapat memiliki halaman berikutnya kosong; tombol sebelumnya tetap tersedia. Offset bukan snapshot lintas halaman: writer lain dapat menggeser hasil, sehingga gunakan muat ulang untuk kondisi terbaru |
| Indikator terlambat | Estimasi telah lewat dan status masih received/processing diberi teks merah. Ready/collected/cancelled tidak dihitung terlambat. Waktu ditampilkan WITA; indikator mengikuti jam perangkat saat pemuatan, tanpa polling |
| Detail | Nama, rincian kg/item dan harga snapshot, subtotal/diskon/total, dibayar bersih/sisa tagihan, versi, estimasi, milestone tersedia serta catatan/alasan batal dirender melalui createElement/textContent. Saldo berasal dari snapshot server; tidak dihitung dari satu halaman history payment. Riwayat pembayaran dapat dimuat berikutnya per 20 |
| Status/izin | Received → processing → ready → collected; pengambilan hanya saat saldo nol. Owner/admin dapat membatalkan received/processing tanpa saldo pembayaran dengan alasan wajib. Payment tersedia pada order terbuka dengan saldo positif. Owner/admin mendapat refund penuh per receipt posted, dengan metode asal dan alasan wajib; cashier tidak mendapat kontrol, dan submit paksa tetap diperiksa. Server tetap memeriksa role aktif, saldo/status dan transaksi di bawah lock |
| Conflict | Status/cancel mengirim expected version. Penolakan 409 menonaktifkan tindakan dan menjelaskan perlunya Muat ulang detail. Refresh mengambil snapshot baru dan mengosongkan input/alasan/pilihan refund; staf meninjau tindakan lagi sebelum key/versi baru dibuat. Tidak ada auto-resubmit dengan versi terbaru. Setelah sukses, aksi juga dikunci sampai refresh eksplisit; kartu antrean diberi pemberitahuan untuk muat ulang |
| Payment/refund | Amount rupiah integer positif, maksimal sisa tagihan; metode cash/bank_transfer/qris. Tunai memerlukan shift aktif pencatat. Satu intent membuat satu key acak kriptografis dan body JSON tetap sebelum kiriman. Submit ganda, penggantian order, refresh dan navigasi/keluar tab yang sama dikunci saat pending. Hasil menampilkan ID receipt, nominal dan apakah replay idempotent ditemukan tanpa posting ulang |
| Hasil tidak pasti | Network/timeout 20 detik/408/5xx/JSON atau respons sukses tidak valid menahan intent. Tombol Periksa / kirim ulang memakai path, actor, key dan byte body sama. Penolakan setelah hasil tidak pasti tetap mengunci intent karena tidak membuktikan percobaan sebelumnya batal. Tidak ada retry/antrean otomatis saat koneksi kembali. Sebelum kiriman pertama, offline tidak membuat transaksi |
| Sesi/draft | Cookie/CSRF memakai modul sesi existing; expiry 401 tetap menuju login tanpa perangkap beforeunload. Key/body hanya di memori tab; reload/penutupan paksa/expiry dapat menghilangkannya. Periksa riwayat order dan transaksi sebelum memasukkan ulang. Tidak ada penyimpanan PII/token ke storage browser |
| Privasi/receipt | `/api/pesanan/{id}/struk?paper=58` atau `80`, default 58, tetap memerlukan sesi dan izin read_orders aktif; GET/HEAD anonim ditolak dan role asing 403. HTML memakai html/template, mencakup rincian/saldo/status/WITA/catatan, tanpa token tracking. Tracking publik existing tetap `/tracking.html#TOKEN` — `/api/tracking/TOKEN`, projection minimal; ID order tidak menjadi kredensial publik |
| Cetak thermal | `receipt.css` memakai lebar fisik 58/80 mm, inset 4/5 mm, monospace hitam 12/13px, wrapping nama/ID/catatan panjang dan item tidak dipotong bila muat satu halaman. Tombol/navigasi disembunyikan pada print. Pilih kertas driver yang sama, skala 100% dan matikan header/footer browser; margin halaman CSS nol, inset menjaga area cetak. Pemilihan ukuran tidak mencetak otomatis; tombol cetak memakai script lokal |
| Batas konsistensi | Detail dan history payment adalah GET terpisah, sehingga transaksi lain tetap dapat terjadi sesudah pembacaan. Server memutuskan payment/refund tanpa expected version di bawah lock; status/cancel memakai version. UI tidak mengklaim snapshot seluruh tab atomik. Angka di luar integer aman JavaScript ditolak sebelum kontrol diaktifkan; Go/receipt tetap memakai int64 |

## Verifikasi Hari 21 — tindakan — hasil

- `node --test --test-name-pattern=queue scripts/frontend-contract.test.cjs` — tiga test lulus untuk graph tindakan/izin, batas pembayaran, overdue dan snapshot invalid; test frontend lain tidak dijalankan.
- `go test ./internal/http/handler ./internal/http -run 'TestQueue|TestCustomerHTTPMinimumDataSearchAndHistory|TestServiceFailuresAreRedactedAndReceiptIsEscaped|TestListRejectsAmbiguousOrUnboundedQueries|TestCustomerAndReceiptAuthorization|TestTrackingPublicPrivacyAndInvalidTokens|TestPaymentHTTPIdempotencyContract|TestOrderCommandHTTPKeys' -count=1 -timeout=60s` — lulus; search/query invalid, saldo detail, thermal 58/80, escaping pelanggan/layanan/catatan, no-store, otorisasi receipt dan token publik. Test receipt dapat mengekspor HTML sintetis untuk review bila `ULAUNDRY_RECEIPT_PREVIEW_DIR` menunjuk direktori cache yang sudah ada.
- `go test ./internal/service -run '^TestPostgres(QueueSearch|OrderConcurrency|FinancePaymentsAndRefunds)$' -count=1 -timeout=120s -v` dengan `ULAUNDRY_TEST_ADMIN_DSN` khusus cluster test — tiga suite lulus (17 subtest workflow/finance). Helper membuat/menghapus DB disposable. Search gabungan status/pagination, stale version, duplicate/replay, payment versus cancel/pickup, refund, constraint dan rollback ledger/audit diperiksa; tanpa database operasional atau `.env`.
- Edge headless dengan API mock sintetis dan modul sesi/UI nyata — search/filter lima status, pagination/halaman kosong, overdue, detail markup aman, 409 dan refresh, double-submit, payment lost-response/timeout lalu replay byte-identik, 403 sesudah hasil tidak pasti, refund permission/replay, cancel dan pelunasan sebelum pickup lulus. Offline tidak mengantre/mengirim otomatis, gagal muat/retry dan 401 tanpa dialog terjebak diperiksa.
- Browser/print — keyboard Enter/fokus detail dan viewport 1280/768/360/390/412px tanpa overflow lulus; screenshot antrean/detail desktop/ponsel ditinjau. HTML struk asli dari handler test dirender dengan CSS/script nyata; preview print dan PDF 58/80 mm, termasuk nama tanpa spasi dan catatan panjang, diperiksa. Kontrol cetak tersembunyi, markup tetap teks, nominal/WITA terbaca; tidak ada exception JavaScript atau console.error.
- Syntax/aset/build — sepuluh HTML dan lima belas JavaScript, UTF-8, aset lokal, ID unik, tokens CSS dan header tabel valid; build `cmd/server` berhasil. Diff/status Git ditinjau dan diff whitespace bersih. Tidak menjalankan full backend/frontend suite, race atau migrasi ulang karena transaction/domain command/schema tidak berubah.
- Batas bukti — browser memakai API mock, PostgreSQL diuji terpisah; belum acceptance browser → server → database nyata, Android/printer fisik atau driver printer produksi. Artefak review sintetis/harness di cache luar repository; lihat [laporan Hari 21](progress/day-21.md). Ini bukan rilis produksi.

## Verifikasi visual terarah

Periksa halaman pada desktop 1280px, tablet 768px dan viewport Android 360/390/412px. Gunakan data sintetis/lingkungan development; jangan memasukkan PII produksi dalam screenshot. Fondasi sembilan halaman diperiksa Hari 18; halaman POS ditambahkan Hari 20.

- Palet/wordmark sesuai baseline; halaman tidak overflow, tabel dapat digeser dengan keyboard/sentuh, label dan kontrol terbaca.
- Menu ponsel, aria-current, Escape/fokus, skip link/Tab, toggle kata sandi dengan Enter dan target 44px.
- Loading, kosong, gagal/retry, toast/tutup, koneksi offline/online; jangan menampilkan angka/dashboard lama saat gagal.
- Role menu, guard dashboard kasir dan keluar dengan cookie/CSRF; ini pemeriksaan UI, bukan pengganti tes authorization server.
- Print media kas/shift menyembunyikan shell/kontrol; reduced-motion mematikan animasi spinner CSS.
- Console/JavaScript dan resource error pada alur normal; respons API gagal yang sengaja disimulasikan dibedakan dari exception UI.

Perintah frontend resmi tetap `node --test scripts/frontend-contract.test.cjs`; Hari 18 menjalankan 14 test, Hari 19 menjadi 22 dan Hari 20 menjadi 29; Hari 21 mengganti satu test antrean lama dengan tiga test model (31 tersedia, hanya tiga test antrean dijalankan pada tugas ini). Syntax dapat diperiksa dengan `node --check` pada file JS yang berubah. Bukti browser memakai Edge headless/API mock; Hari 21 menjalankan HTTP dan PostgreSQL terarah secara terpisah. Android fisik, printer fisik dan acceptance database end-to-end bukan bukti tugas ini. Bukti fondasi ada pada [laporan Hari 18](progress/day-18.md), auth/staf pada [Hari 19](progress/day-19.md), POS pada [Hari 20](progress/day-20.md) dan antrean/transaksi/receipt pada [Hari 21](progress/day-21.md).
