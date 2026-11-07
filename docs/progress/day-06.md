# Laporan Hari 06 — Repository PostgreSQL dan Batas Transaksi

## Tindakan — Hasil

- Kontrak repository berbasis use case — lookup/registrasi customer, katalog layanan, order query/progress, payment-ledger, expense, user/credential dan audit tersedia tanpa CRUD generik atau dependency SQL/HTTP pada kontrak.
- Adapter PostgreSQL — seluruh operasi baru memakai context/bind parameter, scan/iterator/close handling, typed not-found/conflict/constraint/invalid-data, tanpa membuka driver detail/cause ke handler atau global DB.
- UnitOfWork — View read-only REPEATABLE READ dan callback transaksi READ COMMITTED memakai satu sql.Tx untuk beberapa repository; commit/rollback error diperiksa dan handle tidak berlaku setelah callback.
- Penyimpanan domain — migration additive 000003 menambahkan delapan tabel BIGINT/snapshot/FK/check/unique/version/indeks serta pembatasan UPDATE/DELETE audit; dua migration terdahulu dan data legacy tetap.
- Order/keuangan — rehidrasi memvalidasi ulang snapshot/histori; FOR UPDATE dan version melindungi perubahan, payment-ledger diposting bersama, replay key identik tidak menggandakan uang dan payload berbeda ditolak.
- Filter/pagination — ListOrders memakai WHERE status/customer dan LIMIT/OFFSET parameter dengan urutan deterministik; filter legacy aktif diperbaiki menjadi WHERE status = $1, QueryContext, hasil kosong [] dan kegagalan baca aman.

## Verifikasi

- `go test ./internal/repository/... -count=1 -timeout=120s -v` — 13 skenario PostgreSQL 16.14 terisolasi lulus: create/read snapshot/BIGINT, katalog/customer/user/audit, filter/pagination/literal search, constraint/not-found/conflict, rollback lintas repository dan late ledger failure, concurrency/replay/overpayment, cancellation row lock, invalid snapshot, scan/iterator/lifetime dan presisi mikrodetik.
- `go test ./internal/migrate -count=1 -timeout=120s` — 18 skenario integration serta unit katalog/redaction lulus; fresh/upgrade legacy dan versi 2→3 mempertahankan data/history/sequence lama, demo menolak DB dengan data domain.
- `go test ./internal/domain ./internal/http/... ./internal/app -count=1 -timeout=45s` — lulus; rehidrasi snapshot dan 11 kasus korupsi, unit domain sebelumnya, regresi binding/context/filter/read failures serta wiring yang terdampak tervalidasi.
- `gofmt`, `go build ./...`, `go vet ./internal/domain ./internal/repository/... ./internal/migrate ./internal/http/handler ./internal/testutil` — berhasil; tanpa dependency baru, UI, full endpoint suite atau full release/race checks.
- Pemeriksaan Git diff/status, SQL LF, dependency direction, tautan dokumentasi dan cleanup DB test — dilakukan pada workspace; `.env`/database pengguna tidak digunakan.
- Cleanup — tidak ada database test tersisa, cluster test dihentikan; chime `sounds/chime-1.mp3` menyelesaikan playback dua kali.

## Perubahan Penting

- `internal/repository/`, `internal/repository/postgres/` — kontrak, boundary transaksi, adapter, query terarah dan PostgreSQL integration tests.
- `internal/domain/order_state.go`, tests — snapshot/rehidrasi dengan validasi uang/status/histori dan salinan data.
- `migrations/000003_domain_storage.up.sql`, migration tests, `scripts/demo.sql`, `internal/testutil/` — storage additive, verifikasi upgrade dan database test acak dengan cleanup.
- `internal/http/handler/` — perbaikan filter legacy terarah; route lain tetap pada implementasi sebelumnya.
- `docs/architecture.md`, README/database/business-rules/arsitektur target/backlog/audit — arah dependency, command, policy transaksi/error/pagination dan rencana cutover diperbarui.

## Risiko atau Sisa Pekerjaan

- Adapter baru belum dipakai service/server. Uang/auth/create/daftar umum legacy tetap memakai SQL lama; risiko total frontend, posting ganda dan privacy baseline belum ditutup oleh test adapter baru.
- Tabel domain baru tidak mengimpor legacy/akun secara otomatis. Sebelum aktivasi wajib rencana backup/restore salinan, mapping status/akun, rekonsiliasi dan satu writer; jangan dual-write atau melakukan rollback destruktif tanpa persetujuan pemilik.
- Auth/izin actor, audit pada use case nyata, password policy/secure bootstrap, refund/report/shift dan edit katalog/profil lengkap masih pekerjaan bertahap berikutnya. SQL manual di luar adapter harus mengikuti locking/invarian yang sama.
- Bukti terbatas pada PostgreSQL 16.14 test; role/TLS/deployment/restore operasional, volume produksi, dan alur UI belum diverifikasi. Tidak ada klaim siap produksi.
