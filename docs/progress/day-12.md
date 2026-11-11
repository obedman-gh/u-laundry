# Laporan Hari 12 — Layanan, Harga dan Snapshot Order

## Tindakan — Hasil

- Katalog existing — pengelolaan owner/admin melalui `/services.html` dan API mencakup nama, kg/item, harga rupiah, estimasi menit, aktif dan urutan; cashier hanya membaca daftar aktif. Histori menampilkan revisi lengkap, actor/waktu dan sumber baseline/staff/demo.
- Validasi harga — unit_price wajib eksplisit pada create/update; hilang atau null ditolak 400, Rp0 tetap sah. Pembulatan existing dipertahankan: gram integer, half-up per baris, subtotal dijumlahkan setelah pembulatan, diskon nominal 0..subtotal dengan persetujuan owner/admin dan alasan untuk diskon positif; total minimum Rp0.
- Snapshot — server mengambil dan mengunci katalog sebelum menghitung nama/satuan/harga/jumlah/total baris serta subtotal/diskon/total. Bukti unit dan PostgreSQL memastikan perubahan harga, satuan, nama, durasi atau status aktif hanya berlaku pada order baru; lima snapshot order yang diterima tetap sama setelah dibaca ulang.
- Histori dan transaksi — konfigurasi, revisi dan audit disimpan atomik dengan row lock/version. Audit gagal membatalkan harga/revisi; stale version ditolak. Editor harga menunggu order yang sudah mengunci layanan; order berikutnya memakai harga setelah commit editor.
- Demo — `scripts/catalog-demo.sql` terpisah dari migration, berisi tiga layanan dan revisi sintetis tanpa akun, pelanggan atau transaksi. Database non-demo dan pengulangan seed ditolak. Migration 000008 existing dipertahankan; baseline bukan rekonstruksi harga sebelum upgrade.

## Verifikasi

- `go test ./internal/domain ./internal/service ./internal/http/handler ./internal/migrate -run 'TestCatalog|TestLineCalculation|TestQuantityAndMoneyFailures|TestWeightAgainstExactArithmetic|TestOrderTotalsAndSnapshots|TestOrderValidation|TestCreateOrder' -count=1 -timeout=60s` — lulus; kalkulasi integer/overflow, pembulatan per baris, batas quantity/weight/diskon, minimum nol, snapshot baru/lama, izin katalog, rollback dan HTTP harga wajib/Rp0 serta respons snapshot lengkap.
- `go test ./internal/service -run '^TestPostgresCatalog' -count=1 -timeout=120s -v` — lulus pada PostgreSQL 16.14, cluster test baru loopback 55443 dengan ULAUNDRY_TEST_ADMIN_DSN; upgrade 7→8 dua kali/history tetap, baseline, revisi/harga baru, snapshot lama, lock order/editor, audit rollback, constraint jumlah/nominal/diskon/total, append-only/unik histori, nonaktif, pengurutan/pagination dan seed demo. Database test disposable dibuat/dihapus helper; tidak membaca .env atau memakai database operasional.
- `node --test --test-name-pattern=catalog scripts/frontend-contract.test.cjs` — 3/3 lulus; role, render teks aman, create dengan CSRF, integer aman dan konflik versi. Test auth tidak dijalankan.
- `go vet ./internal/domain ./internal/service ./internal/repository/postgres ./internal/http/dto ./internal/http/handler` dan `go build ./cmd/server ./cmd/migrate` — lulus. gofmt pada file Go yang berubah; diff/status dan dokumen diperiksa. Cache Go memakai .cache workspace.
- Cleanup — 0 database test disposable tersisa; cluster test dihentikan. UTF-8 dan tautan lokal dokumen valid; git diff --check bersih.

## Perubahan Penting

- internal/http/dto/requests.go dan handler/workflows.go — field harga pointer tervalidasi membedakan harga hilang/null dari Rp0 eksplisit.
- internal/service/catalog_test.go, catalog_integration_test.go dan internal/http/handler/catalog_test.go — bukti kalkulasi multi-baris/snapshot, nilai order saat harga diedit bersamaan, rehidrasi lima snapshot, constraint diskon/jumlah serta total nol.
- docs/business-rules.md, api.md, architecture.md, database.md dan README — aturan dan kontrak katalog/histori/snapshot, command terarah, seed dan panduan rollback diperbarui.

## Risiko atau Sisa Pekerjaan

- Migration/deployment pada database operasional belum dijalankan. Riwayat harga sebelum baseline tidak tersedia; rollback harus memakai writer kompatibel histori versi 8 atau forward fix.
- UI diuji dengan DOM sintetis; browser nyata/Android dan printer belum diverifikasi. Aturan minimum Rp0 dan pembulatan existing tetap perlu ditinjau sebelum penggunaan operasional. Tidak ada blocker implementasi yang diketahui.
