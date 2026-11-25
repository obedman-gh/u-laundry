# Audit trail — Hari 17

Audit operasional dan keamanan tersedia di `/audit.html` dan `GET /api/audit/events`, **khusus owner aktif**. Admin/cashier tidak dapat membaca audit, termasuk endpoint objek lama `/api/audit`. Tidak ada endpoint menambah, mengedit, menghapus atau membersihkan audit.

## Event dan transaksi

| Sumber | Aksi | Jaminan |
|---|---|---|
| audit_events | Bootstrap/create/activate/deactivate/reset password/perubahan role staf | Mutasi akun, pencabutan sesi dan audit atomik |
| audit_events | Konfigurasi layanan/harga/durasi/aktif/urutan | Konfigurasi, revisi katalog dan audit atomik |
| audit_events | Customer; create/status/cancel order; diskon; rotasi tracking | Audit ikut transaksi bisnis; replay command tidak menambah event |
| audit_events | Pembayaran/refund dan buka/mutasi/tutup shift | Receipt/ledger/rekonsiliasi dan audit atomik |
| audit_events | Pembuatan laporan | Audit commit sebelum respons; snapshot sesuai reports.md |
| audit_events | credentials_verified | Credential valid dan token ditandatangani; belum berarti sesi login berhasil |
| auth_events | login_success | Atomik bersama registrasi sesi; audit gagal berarti tidak ada sesi/cookie baru |
| auth_events | logout | Atomik bersama revocation; audit gagal membatalkan revocation |
| auth_events | login_failure/login_limited/access_denied | Transaksi terpisah setelah penolakan; bukan audit sukses bisnis |

Konfigurasi aplikasi yang sudah dapat dimutasi adalah katalog dan akses staf; semuanya melewati use case di atas. `manage_configuration` masih policy cadangan. Perubahan environment/secret deployment di luar aplikasi tidak otomatis dipantau atau disalin ke audit.

## Field dan privasi

Event memuat source/id, actor ID, action, entity/target type dan ID, occurred_at, request_id dan metadata minimum. Gunakan pasangan source/id untuk identitas event lintas tabel. Actor adalah pelaku; entity_id adalah objek yang diubah. Security event dengan actor diketahui diproyeksikan menjadi target staff/actor ID. Login gagal/limited anonim memakai actor/target ID kosong dan entity authentication; username percobaan tidak disimpan dan identitas tidak dikarang.

Middleware membuat request ID acak server; header X-Request-ID caller tidak dipercaya. Adapter menyimpan korelasi yang sama dengan header respons. Bootstrap/command non-HTTP mendapat ID acak saat menulis audit. Event bisnis sebelum schema 12 tetap request_id NULL dan metadata `{}`; API menyajikan request_id kosong. Tidak ada rekonstruksi korelasi historis.

Metadata bisnis bertipe, dengan allowlist yang diperiksa domain dan constraint JSONB PostgreSQL:

- `version`: versi hasil mutasi staf/layanan/order, integer positif bila ada.
- `status`: enum status order hasil mutasi.
- `role`: enum role hasil create/bootstrap/perubahan role.
- `active`: boolean status staf/layanan hasil mutasi.
- `unit_price`: harga layanan integer rupiah, termasuk nol.

Event lain menggunakan `{}`; konfigurasi lain dapat ditelusuri lewat target ID/versi ke histori katalog privat. Security access_denied membawa permission dari allowlist database. Tidak ada password/hash, JWT, token CSRF/tracking, session hash/fingerprint, secret/DSN, payload request/response, nama/kontak/alamat/catatan pelanggan, alasan refund/penutupan atau nama layanan. Reader tidak melakukan join nama staf/customer.

Marker staff_logged_in historis dipertahankan dengan arti verifikasi credential lama. Kode Hari 17 menulis credentials_verified; **login_success** adalah bukti registrasi sesi. Jika tahap registrasi gagal, event verifikasi yang benar dapat tetap ada, tetapi tidak ada login_success/sesi/cookie palsu. Audit sukses bisnis tidak bertahan ketika transaksi bisnis rollback.

## Append-only dan akses database

Migration **000012** menambah request_id, metadata, indeks waktu/aksi/korelasi dan trigger penolakan TRUNCATE untuk audit_events/auth_events. Trigger UPDATE/DELETE existing tetap. INSERT bisnis baru wajib request ID valid. Histori lama dan SQL 1–11 tidak diubah.

Guard berlaku pada jalur SQL biasa; superuser/pemilik DDL dapat menonaktifkan trigger atau menjatuhkan tabel. Deployment harus memakai role runtime bukan superuser/pemilik: audit hanya SELECT/INSERT, tanpa UPDATE/DELETE/TRUNCATE/DDL; sequence identity auth_events tetap membutuhkan hak yang sesuai. Role staf aplikasi tidak identik dengan role PostgreSQL. Hak runtime produksi belum dibuktikan pada infrastruktur nyata.

## Filter, halaman dan API

```text
GET /api/audit/events
GET /api/audit/events?action=payment_recorded&limit=20&offset=0
GET /api/audit/events?source=security&action=login_failure
GET /api/audit/events?from=2026-11-01T00:00:00%2B08:00&to=2026-11-26T00:00:00%2B08:00
GET /api/audit/events?entity=shift&entity_id=SHIFT_ID
GET /api/audit/events?request_id=SERVER_REQUEST_ID
```

Filter opsional: from/to RFC3339 berpasangan, action exact, source business/security, actor_id, entity, entity_id, request_id, limit dan offset. Entity bisnis staff/customer/service/order/payment/refund/shift/report, serta authentication untuk keamanan anonim. Field asing/duplikat, enum/ID invalid, tanggal tanpa zona atau periode tidak lengkap ditolak 400.

Default 30 hari kalender termasuk hari ini di WITA; maksimum 366 hari. Awal inklusif dan akhir eksklusif. UI menafsirkan tanggal akhir sebagai hari inklusif lalu mengubahnya ke awal hari berikutnya. Respons `from/to/timezone/limit/offset/has_more/items`. Limit default 20, maksimum 100; offset minimal nol. Hasil kosong `items: []`.

Urutan `occurred_at DESC,source DESC,id DESC` stabil ketika timestamp sama. Query UNION ALL memproyeksikan dua tabel tanpa menyalin histori; seluruh filter memakai bind parameter. Satu request repeatable-read; halaman terpisah bukan snapshot beku dan event baru dapat menggeser offset. Untuk pemeriksaan konsisten gunakan rentang waktu yang sudah selesai.

Cookie sesi dan role/active terkini diperiksa server. Tanpa sesi 401; admin/cashier 403 meski claim lama menyebut owner; metode selain GET/HEAD 405. Respons privat no-store/nosniff. Endpoint objek lama tetap array bisnis, sekarang owner-only dengan request_id/metadata. UI memakai DOM/textContent, label, tabel geser pada ponsel, fokus terlihat dan loading/empty/offline/error; tanpa localStorage token atau offline write queue.

## Retensi operasional

Kebijakan awal: **minimum 24 bulan audit bisnis dan 90 hari audit keamanan tersedia untuk pemeriksaan**. Credential terverifikasi tetap kategori bisnis. Owner meninjau volume setiap tiga bulan dan setelah insiden. Event insiden dipertahankan sampai pemeriksaan selesai walaupun melewati periode biasa.

Arsip bulanan privat: backup terenkripsi ke media owner, akses terbatas owner/operator backup, rentang waktu/count dan checksum SHA-256 tercatat, serta pembacaan/restore ke database baru diverifikasi. Simpan arsip minimal 24 bulan; periksa kapasitas dan keberhasilannya setiap bulan. Jangan simpan dump di Git, frontend, URL publik atau log troubleshooting. Backup biasa bukan otomatis arsip terverifikasi; runbook backup/restore teruji tetap pekerjaan operasi berikutnya.

Belum ada penghapusan otomatis, scheduler, purge API atau perintah menonaktifkan trigger. Periode di atas adalah minimum retensi, bukan klaim data pasti hilang sesudahnya. Histori saat ini tetap append-only. Pengurangan/arsip histori setelah periode tersebut harus menjadi maintenance terpisah yang ditinjau owner, mempunyai backup/restore terverifikasi dan otorisasi eksplisit sebelum perubahan data nyata. Jangan memakai DELETE/TRUNCATE rutin atau mematikan guard untuk mengejar ukuran tabel. Ini kebijakan operasional proyek, bukan penetapan retensi hukum.

## Upgrade dan rollback

Hentikan writer, backup dan uji restore salinan, lalu runner resmi `status`, `up`, `status` ke schema 12. Kolom lama tetap; request ID lama tidak diisi. Binary schema ≤11 tidak memasok korelasi dan tidak mengenal credentials_verified, sehingga bukan rollback writer yang kompatibel. Pertahankan schema 12 dengan forward fix/binary kompatibel; tanpa drop kolom/trigger atau perubahan checksum migration lama. Tidak ada migration atau maintenance database operasional dalam pekerjaan Hari 17.

## Verifikasi terarah

Gunakan toolchain/cache README dan ULAUNDRY_TEST_ADMIN_DSN ke database postgres pada cluster test khusus, bukan `.env` operasional. Helper membuat/membersihkan DB acak.

```powershell
go test ./internal/domain ./internal/service ./internal/http/... -run '^(TestAudit|TestPermissionMatrix|TestStaffHTTPPermissionMatrix)' -count=1 -timeout=60s
go test ./internal/migrate -run '^TestPostgresAuditUpgrade$' -count=1 -timeout=60s -v
go test ./internal/service -run '^TestPostgresAuditTrail$' -count=1 -timeout=90s -v
go test ./internal/http -run '^TestPostgres(AuditHTTP|CookieAuthenticationLifecycle)$' -count=1 -timeout=90s -v
node --check frontend/audit.js
```

Test mencakup seluruh kelompok aksi, korelasi/metadata/privasi, enam rollback bisnis, login/session gagal tanpa sukses palsu, filter/pagination, owner/admin/cashier, upgrade 11→12 berulang, histori tetap dan penolakan UPDATE/DELETE/TRUNCATE/payload metadata liar. Bukti hasil dan batas verifikasi di [laporan Hari 17](progress/day-17.md).
