# Database dan migration ULaundry

Status hari 07 (7 November 2026): empat migration forward-only. Versi 4 mendukung workflow service aktif dan diverifikasi melalui satu alur PostgreSQL **16.14** test terisolasi, termasuk upgrade 3→4. Skema operasional/VM belum diperiksa atau dimigrasi. Server tidak menjalankan DDL saat startup. Semua objek aplikasi berada pada schema `public`.

## Keputusan alat

Gunakan runner kecil `cmd/migrate` / `internal/migrate`, `database/sql`, driver `lib/pq` yang sudah ada, dan SQL tertanam dengan `go:embed`. Tidak menambah dependency atau executable eksternal. Skema transaksional saat ini belum membutuhkan framework migration; cakupan runner sengaja dibatasi:

- File `migrations/NNNNNN_description.up.sql`, nomor berurutan dari 000001. Semua file dikemas saat build; tidak bergantung pada cwd untuk SQL.
- `public.schema_migrations`: `version` PK, `name` unik, SHA-256 isi file, `applied_at` TIMESTAMPTZ. `.gitattributes` menjaga LF agar checksum Windows/Ubuntu konsisten.
- Seluruh versi pending, DDL dan history masuk **satu transaksi**. Gagal pada versi kedua juga membatalkan versi pertama dalam pemanggilan yang sama. Versi yang sudah committed sebelumnya tetap utuh. Tidak ada status dirty terpisah karena DDL yang didukung transaksional.
- `pg_try_advisory_xact_lock` memakai kunci tetap per database; runner kedua gagal cepat. Lock otomatis lepas ketika transaksi selesai/koneksi putus. Ini hanya koordinasi antar-runner; tetap hentikan penulisan aplikasi saat upgrade.
- Lock tabel dibatasi 5s, seluruh command 2m secara default, dapat diperbesar melalui `-timeout`. Pembatalan/timeout menyebabkan rollback. Jika hasil commit tidak diketahui karena koneksi putus, jalankan `status` melalui koneksi baru sebelum retry.
- `status` memakai transaksi read-only dengan snapshot konsisten; tidak membuat history pada DB fresh. `up`/`status` menolak checksum/nama berubah, urutan berlubang atau versi database lebih baru dari executable.
- Error DB hanya menampilkan operasi/versi, SQLSTATE dan nama constraint; tidak mencetak row, username, password atau DSN.

Tambahkan versi baru untuk setiap perubahan; jangan mengubah SQL atau history yang sudah diterapkan. Tidak ada `force`, baseline-stamp atau `down`. Runner tidak mendeteksi semua perubahan DDL manual setelah migration dan bukan alat schema-diff. Jangan menambah `CREATE INDEX CONCURRENTLY`, operasi nontransaksional atau transaksi eksplisit ke SQL ini; evaluasi runner/library lain jika kebutuhan berkembang.

Dasar PostgreSQL: [transaction advisory locks dan table locks](https://www.postgresql.org/docs/16/explicit-locking.html), [CHECK/NOT NULL/UNIQUE/FK](https://www.postgresql.org/docs/16/ddl-constraints.html). FK tidak otomatis mengindeks sisi anak; indeks relasi ledger dibuat eksplisit.

## Versi dan kompatibilitas

| Versi | Perubahan | Dampak pada data lama |
|---|---|---|
| 000001 | Membuat/adopsi tiga tabel lama dan kolom waktu pesanan yang pernah ditambah utilitas informal | Tidak mengganti tabel, ID, sequence atau row. Kolom `waktu_pesan` yang sebelumnya tidak ada mendapat waktu adopsi; bukan waktu historis transaksi |
| 000002 | Constraint uang/ledger/username/waktu, metadata update, FK opsional dan indeks | Data invalid menggagalkan transaksi. Metadata baru terisi waktu adopsi; tidak merekonstruksi history |
| 000003 | Tabel domain customers/services/staff/orders/order_items/payments/ledger_entries/audit_events, BIGINT, FK/check/unique/version dan indeks | Additive, tanpa import otomatis atau perubahan history/row/sequence legacy; fondasi adapter hari 06, aktif pada server hari 07 setelah versi 4 |
| 000004 | Version layanan, hash token tracking, superset allowed audit actions dan indeks rentang ledger | Tidak menghapus/mengubah row legacy; audit constraint divalidasi ulang dalam transaksi. Server memakai service/domain storage, sehingga akun/data legacy memerlukan import/cutover tersendiri |

`total_biaya` dan `nominal` tetap PostgreSQL `INTEGER` (int4), satuan rupiah, rentang 0..2.147.483.647; tidak mengonversi menjadi float. Tipe monetary legacy lain ditolak untuk pemeriksaan manual. Tidak ada konversi otomatis timestamp lama: `pesanan.waktu_pesan`, perkiraan/selesai dan `keuangan.tanggal` tetap `TIMESTAMP WITHOUT TIME ZONE`, beserta nilainya. Jangan mengasumsikan timezone historis; migrasi timezone memerlukan keputusan terpisah.

Storage versi 3 memakai BIGINT rupiah utuh dan ID varchar(100), sesuai domain int64; tidak memperlebar kolom legacy. Item/customer menyimpan snapshot terpisah dari katalog/customer aktif. Order memiliki version untuk optimistic conflict; payment/ledger memiliki key unik dan FK komposit yang melindungi referensi/amount. Timestamps baru TIMESTAMPTZ; adapter menormalisasi UTC dan truncates ke mikrodetik. Audit UPDATE/DELETE ditolak trigger; privilege TRUNCATE/DDL tidak boleh diberikan ke role aplikasi. Detail query/transaksi/error dan batas perlindungan ada di [architecture.md](architecture.md).

Versi 4 menyimpan SHA-256 token tracking pada BYTEA unik 32 byte dengan FK order; raw token hanya diberikan sekali sesudah commit, tidak dapat diambil kembali dari DB. Tidak ada expiry/rotation saat ini. Action audit lama tetap diterima setelah constraint diperluas. Configuration layanan memakai expected version dan lock; report query memakai indeks occurred_at/type. Tabel layanan/customer/order/payment lama pada schema domain tidak diduplikasi; tabel legacy tetap terpisah.

Constraint versi 2:

- `pesanan.total_biaya` dan `keuangan.nominal` NOT NULL dan >= 0; nol diterima.
- `keuangan.tipe` NOT NULL dan hanya `DEBIT`/`KREDIT` (case-sensitive).
- `kasir.username` NOT NULL dan UNIQUE; memakai constraint lama jika sudah ada, tanpa indeks duplikat. Semantik username tetap case-sensitive.
- `waktu_pesan` dan `tanggal` NOT NULL + default CURRENT_TIMESTAMP. NULL lama ditolak, tidak diisi diam-diam. Perkiraan/selesai tetap nullable.
- `kasir.created_at`, `updated_at` di ketiga tabel: TIMESTAMPTZ NOT NULL, default CURRENT_TIMESTAMP; trigger BEFORE UPDATE mengisi `updated_at` melalui `clock_timestamp()`. Created time pesanan/ledger memakai kolom lama. Ini metadata, bukan audit trail append-only.
- `keuangan.id_pesanan` nullable, FK ke PK `pesanan.id_pesanan`, ON DELETE/UPDATE RESTRICT; tidak ada cascade yang menghapus uang. Pengeluaran boleh tanpa pesanan. Ledger historis tidak dihubungkan dengan menebak isi `keterangan`.

Handler pembayaran lama masih tidak mengisi `id_pesanan`. Constraint hanya melindungi relasi yang diisi; pengisian relasi aplikasi dan idempotency key/financial posting concurrency menjadi pekerjaan repository/payment berikutnya. Tidak menambahkan UNIQUE order-ledger karena aturan payment/refund belum disepakati. `status` pesanan belum dibatasi/diberi default: workflow domain hari 05 akan menetapkan nilai/transisi; create handler lama masih belum mengisi status wajib.

### Indeks dari query nyata

| Indeks | Query/alasan |
|---|---|
| PK `pesanan.id_pesanan` | Detail/struk dan lookup sebelum update: `WHERE id_pesanan=$1` |
| UNIQUE `kasir.username` | Login: `WHERE username=$1` |
| `keuangan_tipe_nominal_idx (tipe) INCLUDE (nominal)` | Dua agregasi GetNeraca: `SUM(nominal) WHERE tipe='DEBIT'/'KREDIT'`; planner dapat memilih seq scan pada data kecil/selektivitas rendah |
| `keuangan_id_pesanan_idx`, partial non-NULL | Pencarian row ledger saat FK memeriksa perubahan/penghapusan parent; menghindari scan ledger penuh |

Tidak menambahkan indeks `status`/tanggal spekulatif: handler filter saat ini belum menggunakan WHERE status, daftar belum memiliki pagination/sort. Belum ada klaim peningkatan performa/EXPLAIN pada volume produksi.

## Command resmi

Jalankan dari root repository dengan konfigurasi DB deployment yang tepat. Migration tidak membuat database/role; role migration memerlukan CONNECT, CREATE schema public/objek, serta kepemilikan tabel lama untuk ALTER, bukan sekadar hak DML. Gunakan role aplikasi dengan hak minimum saat menjalankan server. Jangan mengandalkan nama DB untuk menentukan production: selalu set `APP_ENV=production` pada operasi production.

```powershell
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/migrate status
# Untuk durasi lebih panjang; flag harus sebelum subcommand.
go run ./cmd/migrate -timeout 5m up
```

`APP_ENV`, DB_DSN atau komponen DB_*, TLS, pool, connect/ping timeout memakai validasi yang sama dengan server. JWT/cookie/HTTP setting tidak diperlukan. Environment mengungguli `.env`; jangan menimpa `.env` yang sudah ada. Lihat `.env.example` dan README untuk aturan DSN/SSL production. CLI tidak menerima password melalui argumen dan tidak mencetak konfigurasi. `status` menghasilkan misalnya:

```text
000001  pending  000001_legacy_baseline.up.sql
000002  pending  000002_integrity.up.sql
000003  pending  000003_domain_storage.up.sql
000004  pending  000004_workflow_support.up.sql
```

Setelah `up`, masing-masing menjadi `applied <waktu UTC>`. No-op `up` tidak mengubah `applied_at` atau data. Exit 0 berarti command selesai; pending pada `status` bukan error. Exit nonzero berarti periksa konfigurasi, konektivitas, versi atau constraint. Jangan menjalankan file SQL migration langsung melalui psql karena itu melewati history/checksum/lock.

Untuk Ubuntu: build `go build -o ulaundry-migrate ./cmd/migrate`, jalankan binary dengan environment deployment yang tervalidasi. SQL sudah tertanam. Aplikasi dan runner harus berasal dari release yang cocok; executable lama menolak history yang lebih baru. Deployment nyata dan hak role production belum diverifikasi hari 04.

## Preflight dan upgrade data lama

1. Identifikasi DB/role/schema yang benar tanpa membocorkan credential: psql `SELECT current_database(), current_user, version();` dan `\d+ public.pesanan`, `\d+ public.kasir`, `\d+ public.keuangan`. Skema yang didukung: init lama repository, varian tanpa tiga kolom waktu, atau tabel parsial yang dibuat utilitas lama. Untuk skema berbeda, tinjau diff terlebih dahulu; `IF NOT EXISTS` bukan jaminan kompatibilitas semua skema.
2. Hentikan server/job/penulisan lain. Simpan versi binary/config dan backup di lokasi privat di luar Git. Backup/restore rehearsal wajib sebelum perubahan data nyata. Jangan memakai database operasional untuk test.
3. Jalankan pemeriksaan di bawah pada tabel/kolom yang sudah ada. Tinjau hasil berupa jumlah; jangan menaruh PII di log/laporan.
4. Restore backup ke database baru terisolasi, arahkan konfigurasi migration ke salinan itu, jalankan `status`, `up`, `status`. Bandingkan row count, ID, sequence, total uang dan waktu lama; verifikasi constraint/route yang menggunakan DB.
5. Setelah salinan lulus dan backup terverifikasi, jalankan `up` di target yang telah dipastikan, cek status serta smoke operasional sebelum membuka penulisan lagi. Tidak ada operasi ini terhadap data pengguna pada hari 04.

```sql
SELECT count(*) AS invalid_order_money FROM public.pesanan
WHERE total_biaya IS NULL OR total_biaya < 0;
SELECT count(*) AS invalid_ledger FROM public.keuangan
WHERE nominal IS NULL OR nominal < 0 OR tipe IS NULL OR tipe NOT IN ('DEBIT','KREDIT');
SELECT count(*) AS duplicate_username_groups FROM
  (SELECT username FROM public.kasir GROUP BY username HAVING count(*) > 1) d;
SELECT count(*) AS null_usernames FROM public.kasir WHERE username IS NULL;
-- Hanya untuk skema yang sudah mempunyai kolom waktu:
SELECT count(*) AS missing_order_time FROM public.pesanan WHERE waktu_pesan IS NULL;
SELECT count(*) AS missing_ledger_time FROM public.keuangan WHERE tanggal IS NULL;
```

Jika gagal: transaksi tidak menerapkan perubahan pending atau history. Data lama tetap, termasuk nilai invalid. Jangan memakai ABS/GREATEST, deduplikasi, DELETE, timestamp palsu atau mengubah checksum untuk memaksa lolos. Pemilik perlu menetapkan koreksi berdasarkan bukti bisnis, backup dan persetujuan sebelum perubahan destruktif; simpan audit koreksi terpisah. Setelah koreksi sah/masalah lock selesai, ulangi `status`/`up`. DDL/validasi dapat memindai dan mengunci tabel; jadwalkan maintenance, bukan upgrade saat kasir menulis. Data skala besar membutuhkan evaluasi migrasi bertahap/indeks concurrent terpisah.

## Backup dan rollback aman

Tidak ada automatic down: DROP tabel/kolom akan kehilangan data dan menghapus jejak versi tidak membatalkan DDL. Jangan drop database aktif, menghapus history, atau menjalankan ulang init lama. Perubahan bersifat penambahan dan mempertahankan kolom lama, sehingga rollback aplikasi lebih aman dengan mempertahankan schema maju selama aplikasi lama kompatibel dengan constraint baru. Runner lama menolak history versi 3; gunakan runner sesuai release untuk status/migration. Uji kompatibilitas binary pada salinan terlebih dahulu; aplikasi lama yang menulis nilai invalid akan ditolak.

Rollback aplikasi legacy bukan mekanisme untuk membaca transaksi domain: server lama tidak membaca orders/payments baru. Sebelum cutover nyata wajib rencana import, rekonsiliasi akun/status/uang, satu writer dan recovery transaksi setelah snapshot. Jangan drop tabel versi 3 untuk kembali ke versi 2 atau menghapus record history; data domain baru harus tetap dipertahankan. Tidak ada import, cutover atau destructive rollback pada pekerjaan hari 06.

Contoh command operator (sesuaikan host/user, gunakan prompt password atau credential file privat; `psql`/pg_dump tidak membaca `.env` aplikasi). Pakai pg_dump/restore yang kompatibel dengan versi server. Lokasi backup di luar repository dengan izin akses hanya operator:

```powershell
pg_dump -h DB_HOST -U BACKUP_USER -d ulaundry -Fc -f F:\private-backups\ulaundry-before-day04.dump
# Pastikan exit code pg_dump = 0, lalu simpan checksum pada media backup yang sama.
Get-FileHash F:\private-backups\ulaundry-before-day04.dump -Algorithm SHA256
pg_restore --list F:\private-backups\ulaundry-before-day04.dump
# Database baru, jangan database aktif. Role/owner tujuan perlu disiapkan.
createdb -h TEST_HOST -U RESTORE_USER ulaundry_restore_check
pg_restore -h TEST_HOST -U RESTORE_USER -d ulaundry_restore_check --exit-on-error --no-owner --no-privileges F:\private-backups\ulaundry-before-day04.dump
```

Checksum dan `--list` hanya memeriksa file/TOC; **restore drill** beserta pemeriksaan row count, uang, sequence dan akses aplikasi diperlukan untuk membuktikan pemulihan. Pada Ubuntu gunakan lokasi privat dan `sha256sum` untuk pemeriksaan checksum. Backup mencakup schema/data/history/sequence; role/global cluster perlu ditangani operator bila digunakan. Retention, enkripsi, offsite dan RPO/RTO akan diputuskan pada pekerjaan backup hari 27.

Jika benar-benar perlu kembali ke skema lama: hentikan semua penulisan, simpan backup kondisi terbaru, restore backup sebelum migration ke **database baru**, verifikasi dengan binary lama, lalu alihkan koneksi melalui perubahan konfigurasi yang ditinjau/disetujui pemilik. Jangan menimpa sumber. Transaksi setelah snapshot backup harus direkonsiliasi sebelum cutover; tanpa itu rollback kehilangan transaksi bisnis. Rollback destruktif/cutover data nyata memerlukan backup dan persetujuan. Forward-fix migration baru lebih disukai jika dapat menjaga seluruh transaksi.

## Data contoh dan utilitas lama

- `scripts/demo.sql` di luar katalog skema: hanya DB disposable bernama akhiran `_demo`, setelah empat migration diterapkan dan semua tabel bisnis legacy/domain kosong. Script tetap menulis contoh legacy saja; **bukan data untuk server domain hari 07**, tidak mengisi tabel domain/akun baru. Jalankan opsional `psql -h HOST -U USER -d ulaundry_demo -v ON_ERROR_STOP=1 -f scripts/demo.sql`. Penolakan/retry tidak menambahkan duplikat; script tidak membuat credential.
- `migrations/init.sql` digantikan dan dihapus setelah runner lolos PostgreSQL test; snapshot skema lama untuk test ada di `internal/migrate/testdata/legacy.sql`.
- `cmd/legacy/upgrade-pesanan` dan `setup-keuangan` sekarang exit nonzero dengan arahan pengganti sebelum membuka DB. Tidak lagi ada DDL/pengeluaran contoh dari executable ini.
- `cmd/legacy/setup-kasir` ditandai deprecated, tidak lagi membuat schema, masih bootstrap credential tetap nonproduction; belum diganti sampai command owner aman hari 10. Jangan gunakan pada data nyata. Migration schema tidak membuat akun; fresh install belum menyediakan bootstrap owner aman.

## Verifikasi terarah

Unit/catalog/config tanpa PostgreSQL: `go test ./internal/config ./internal/migrate ./cmd/migrate ./migrations -count=1 -timeout=45s`. Integration di-skip jika env test tidak diisi; skip bukan bukti migration lulus.

```powershell
# Wajib cluster test khusus, admin role dengan CREATEDB, target database postgres.
# Contoh lokal tanpa secret hanya untuk cluster disposable loopback, bukan production.
$env:ULAUNDRY_TEST_ADMIN_DSN = 'postgres://TEST_ADMIN@127.0.0.1:55434/postgres?sslmode=disable'
go test ./internal/migrate -run '^TestPostgresMigrations$' -count=1 -timeout=120s -v
```

Test tidak membaca `.env`, membuat DB `ulaundry_migration_test_<random>` (kadang suffix `_demo`), menutup koneksi dan menghapus hanya DB tersebut; tidak mengubah tabel pada DB admin. Jangan isi DSN production. Test gagal bila pembuatan/cleanup DB gagal. Cakupan: fresh/status read-only, repeat up/history stabil, skema legacy/parsial/tanpa waktu, pending versi 2, snapshot nilai/sequence tetap, penolakan data invalid/tipe float, rollback DDL/history, timestamp trigger, check/unique/FK/restrict, indeks, demo guard/retry, lock contention, timeout/release, checksum/nama/gap/versi future.

Bukti hari 04: seluruh 17 subtest lulus pada cluster baru PostgreSQL 16 di loopback port 55434, di luar repository. Tidak memakai database pengguna, tidak menjalankan UI/full backend suite. Belum membuktikan restore operasional, volume/lock duration production, versi PostgreSQL lain atau ledger aplikasi yang idempotent.

Bukti hari 06: 18 subtest migration lulus, termasuk upgrade versi 2→3 yang mempertahankan history/data legacy. Test repository `go test ./internal/repository/... -count=1 -timeout=120s -v` memakai env cluster test yang sama tetapi membuat database acak `ulaundry_repository_test_*`; 13 skenario meliputi snapshot, filter, constraint, rollback lintas repository, concurrency/replay/overpayment dan error paths. Test tidak membaca `.env`. Adapter baru belum di-wire ke alur kasir; bukti ini tidak menutup risiko payment legacy atau import/cutover/restore nyata.

Bukti hari 07: `go test ./internal/service -run '^TestPostgresPaymentUseCaseAtomicity$' -count=1 -timeout=90s -v` membuktikan satu alur payment-ledger-audit rollback/retry/replay dan upgrade 3→4 tanpa perubahan history/row legacy, termasuk query tracking/report/configuration yang dipakai alur. Tidak mengulang seluruh integration suite/UI. Unit use case menggunakan fake repository tanpa .env/DB. Server sudah di-wire ke tabel domain, tetapi import/cutover/akun bootstrap/restore operasional belum dilakukan; jangan menjalankan deployment lama seolah data telah terkonversi.

## Migration 000005 — sesi dan audit autentikasi

Tambahan staff_sessions (hash jti, staf/role/username, fingerprint credential, expiry/revocation) dan auth_events append-only (actor nullable untuk kegagalan anonim, kind/request_id/waktu), tanpa JWT/password mentah. Trigger perubahan password_hash/role/active/username staf mencabut sesi lama. Migration 000001–000004 tetap immutable; tidak ada import/reset akun atau perubahan ledger.

Bukti hari 09: PostgreSQL 16 pada cluster test baru loopback 55439, upgrade 4→5 dua kali, row legacy dan history tetap, cookie HTTPS login/logout, pencabutan permanen serta audit rollback/no-cookie dan append-only. Test membuat/menghapus DB disposable dan tersisa 0 DB test; cluster dihentikan. Tidak menjalankan migration pada database operasional. Command integration dan hak runtime ada di [security.md](security.md).

Rollback aman mempertahankan tabel/audit versi 5; jangan drop/truncate/reset untuk menjalankan setup. Lakukan backup dan restore salinan sebelum upgrade data nyata. Reaktivasi server lama berbasis Bearer/localStorage membutuhkan penilaian keamanan, bukan downgrade otomatis ketika login gagal. Retensi event/cleanup sesi tidak diotomatisasi pada tahap ini.

## Migration 000006 — pengelolaan staf dan denial audit

Menambah staff.version BIGINT positif (default 1), allowed audit actions staff_created/activated/deactivated/role_changed/password_reset/bootstrapped dan auth_events.permission dengan kind access_denied. Akun, ID, password_hash dan histori existing tetap. SQL 000001–000005 tidak diubah. Bootstrap dan seluruh perubahan staf memakai pg_advisory_xact_lock(7643296101001), berbeda dari lock migration, supaya count/actor/version/update terserialisasi. Acuan: [transaction advisory locks PostgreSQL 16](https://www.postgresql.org/docs/16/explicit-locking.html#ADVISORY-LOCKS).

Bukti terarah hari 10: PostgreSQL 16, bootstrap serentak hanya satu owner, upgrade 5→6 idempotent dengan akun/hash tetap, username unique/role CHECK, stale version dan self-lockout, pencabutan sesi role/reset/active dan rollback audit (termasuk revocation), audit denial bertahan pada pool satu koneksi. Tidak menjalankan integration finansial/UI umum atau mengubah DB operasional. Batas privilege runtime mencakup SELECT/INSERT/UPDATE staff dan tabel/session audit terkait, tanpa hak superuser/DDL runtime. Backup/restore salinan tetap wajib sebelum upgrade nyata; jangan drop/reset akun untuk membuka bootstrap ulang.
