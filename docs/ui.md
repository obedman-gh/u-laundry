# UI ULaundry — Hari 18

Fondasi bersama memakai HTML semantik, CSS dan JavaScript tanpa framework/build frontend. Identitas ULaundry/Perumahan Viola dan palet existing dipertahankan. Sembilan halaman memakai tokens bersama: login, antrean, dashboard, pelanggan, layanan, staf, kas/shift, audit dan tracking. Go, API, konfigurasi dan migration tidak berubah; schema tetap 12.

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
  ui/favicon.svg     favicon lokal dalam palet existing
  login.html/js      login dan toggle kata sandi
  index.html         antrean, URL / tetap
  queue.js           handler antrean existing yang diekstrak dari HTML
  dashboard.html/js  ringkasan awal owner/admin
  customers.*        pelanggan dan histori
  services.*         katalog
  staff.*            pengelolaan akun
  shifts.*           kas/rekonsiliasi dan print overrides
  audit.*            penelusuran audit
  tracking.*         tracking publik
```

Urutan halaman: tokens.css → ui.css → CSS khusus halaman, lalu ui.js → JS halaman, semuanya lokal dan script `defer`. HTML tidak lagi mempunyai blok CSS/JavaScript inline. HTML lama dirapikan tanpa mengganti ID form, URL, enum, payload atau aturan transaksi. Login/antrean sekarang memakai file script terpisah; fixture test frontend mengikuti sumber tersebut.

Server tetap melayani `STATIC_DIR` melalui `http.Dir`; salin seluruh `frontend/`, termasuk `ui/`, sebagai satu set. Aset tidak membutuhkan npm, CDN, font eksternal atau proses build frontend. Folder ini dapat dibungkus `embed.FS` pada pekerjaan packaging berikutnya; embedding Go belum diaktifkan agar konfigurasi/lifecycle backend tidak ikut berubah pada tugas UI ini.

## Kerangka dan komponen

| Komponen | Kontrak |
|---|---|
| Shell staf | `<body data-staff-app data-page="orders">` dengan satu `<main>`. ui.js menambahkan header/wordmark, navigasi utama, identitas sesi, tombol keluar dan skip link. Page keys: orders/dashboard/customers/services/staff/shifts/audit |
| Sesi/navigasi | JS halaman menyerahkan hasil GET session existing ke `ULaundryUI.setSession(session)`; shell tidak melakukan fetch sesi tambahan. Link pemilik/admin/kasir mengikuti role; izin API tetap ditegakkan server |
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
<script src="/halaman.js" defer></script>
<!-- Dalam body data-staff-app data-page yang sesuai -->
<main><section class="card"><h1>Judul halaman</h1></section></main>
```

```javascript
window.ULaundryUI.setSession(session);
window.ULaundryUI.message(document.getElementById('message'), 'Perubahan tersimpan.', 'success');
window.ULaundryUI.state(container, {
  kind: 'error', title: 'Data belum dapat dimuat',
  description: 'Periksa koneksi lalu coba kembali.', retry: load
});
```

Helper hanya membentuk elemen dan textContent, tanpa innerHTML atau template string HTML dari data. Sesi/CSRF hanya di memori seperti alur existing; tidak menambahkan penyimpanan JWT/token di localStorage. Login tetap membersihkan token legacy. Tracking tetap membaca fragmen token tanpa mengirimnya sebagai URL halaman atau mengubah projection publik.

## Batas halaman dashboard

`/dashboard.html` adalah fondasi ringkasan owner/admin: pesanan hari ini, penerimaan, piutang, shift aktif, status antrean dan net setelah refund. Data berasal dari `/api/laporan/dashboard` existing, tanpa nilai demo/fallback dalam source aplikasi. Sebelum data valid tidak ada angka sementara; kegagalan menyembunyikan angka lama dan menawarkan retry. Bilangan harus aman ditampilkan sebagai integer JavaScript.

Kasir yang membuka URL tersebut mendapat petunjuk menuju alur operasional tanpa request laporan sensitif. Halaman tidak melakukan polling; refresh membaca laporan dan mengikuti audit server existing. Filter periode, detail laporan, CSV serta penerimaan UI finansial lengkap tetap Hari 22. Login/session/user management acceptance tetap Hari 19, POS/antrean lengkap Hari 20–21. Hari 18 tidak menutup AC15 atau kesiapan produksi seluruh aplikasi.

## Verifikasi visual terarah

Periksa kesembilan halaman pada desktop 1280px, tablet 768px dan viewport Android 360/390/412px. Gunakan data sintetis/lingkungan development; jangan memasukkan PII produksi dalam screenshot.

- Palet/wordmark sesuai baseline; halaman tidak overflow, tabel dapat digeser dengan keyboard/sentuh, label dan kontrol terbaca.
- Menu ponsel, aria-current, Escape/fokus, skip link/Tab, toggle kata sandi dengan Enter dan target 44px.
- Loading, kosong, gagal/retry, toast/tutup, koneksi offline/online; jangan menampilkan angka/dashboard lama saat gagal.
- Role menu, guard dashboard kasir dan keluar dengan cookie/CSRF; ini pemeriksaan UI, bukan pengganti tes authorization server.
- Print media kas/shift menyembunyikan shell/kontrol; reduced-motion mematikan animasi spinner CSS.
- Console/JavaScript dan resource error pada alur normal; respons API gagal yang sengaja disimulasikan dibedakan dari exception UI.

Perintah frontend resmi tetap `node --test scripts/frontend-contract.test.cjs`; saat Hari 18 dijalankan 14 test. Syntax dapat diperiksa dengan `node --check` pada file JS yang berubah. Bukti visual memakai Edge headless dan API mock lokal, tanpa PostgreSQL atau automated test backend. Android fisik, printer fisik dan acceptance database end-to-end bukan bukti tugas ini. Hasil aktual ada pada [laporan Hari 18](progress/day-18.md).
