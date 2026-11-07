# Laporan Hari 04 — Migration PostgreSQL Berversi dan Integritas Skema

## Tindakan — Hasil

- Runner Go tanpa dependency baru — tersedia `up`/`status`, dua SQL tertanam berurutan, history SHA-256/waktu penerapan, satu transaksi atomik untuk seluruh pending migration, advisory lock dan deadline.
- Adopsi skema lama — tabel/ID/sequence/nilai lama dipertahankan pada fixture init, instalasi parsial dan varian sebelum kolom waktu; data invalid menggagalkan upgrade tanpa koreksi otomatis.
- Integritas database — uang integer non-negatif, ledger DEBIT/KREDIT, username unik, timestamp wajib/metadata update, FK ledger-pesanan opsional dengan RESTRICT, serta dua indeks sesuai agregasi neraca dan pemeriksaan FK.
- Penggantian setup informal — init lama dihapus setelah pengganti terverifikasi; dua executable DDL/demo lama dinonaktifkan; setup-kasir deprecated tanpa DDL; data sintetis terpisah dan dibatasi DB kosong akhiran `_demo`.
- Dokumentasi operasi — keputusan alat, command, preflight/adoption, backup/restore dan rollback aman tersedia; tidak menjalankan migration terhadap database pengguna.

## Verifikasi

- `go test ./internal/migrate -run '^TestPostgresMigrations$' -count=1 -timeout=120s -v` pada cluster PostgreSQL 16.14 khusus test — 17 subtest lulus: fresh, upgrade/pending/parsial, snapshot nilai/sequence, constraint/FK/timestamp/indeks, invalid data/float, rollback, demo guard, lock/timeout dan history drift.
- Targeted tests `internal/config`, `internal/platform`, `internal/http`, `internal/http/middleware`, `internal/app`, `internal/migrate`, `cmd/migrate`, `migrations` — lulus; shared config tetap memenuhi policy sebelumnya, tanpa `.env` developer/DB operasional. Integration command dijalankan terpisah dengan env test eksplisit.
- CLI binary test pada DB disposable — status pending → dua versi applied, status ulang dan up ulang mempertahankan waktu/history; SQL demo opsional berhasil; DB smoke dihapus, tidak ada DB test acak tersisa.
- `gofmt`, `go build ./...`, `go vet ./internal/migrate ./cmd/migrate ./migrations ./internal/config`, pemeriksaan diff/status — berhasil. Tidak menjalankan UI, full suite atau release/race checks.
- Cleanup DB test dan cluster — tidak ada database test tersisa, cluster test dihentikan; chime `sounds/chime-1.mp3` menyelesaikan playback dua kali.

## Perubahan Penting

- `cmd/migrate/`, `internal/migrate/`, `migrations/`, `.gitattributes` — protokol migration, SQL immutable/LF, unit dan PostgreSQL integration test; snapshot schema lama khusus fixture.
- `internal/config/` — pemuatan mode/DB untuk command operasional menggunakan policy DB yang sama, tanpa kewajiban secret HTTP/session.
- `cmd/legacy/`, `scripts/demo.sql` — penghentian DDL informal dan pemisahan data contoh dari skema.
- `README.md`, `docs/database.md`, arsitektur/audit/backlog dan laporan hari 04 — command/status/keputusan serta batas bukti diperbarui.

## Risiko atau Sisa Pekerjaan

- Skema/data operasional, restore drill backup nyata, TLS/role production, PostgreSQL selain 16.14 serta durasi lock/validasi pada volume nyata belum diverifikasi. Backup, rehearsal pada salinan dan persetujuan wajib sebelum operasi destruktif; tidak tersedia automatic down.
- Data lama yang negatif/NULL/duplikat/tipe ledger asing membutuhkan keputusan koreksi pemilik. Timestamp metadata baru/waktu_pesan yang sebelumnya tidak ada memakai waktu adopsi; timezone historis tidak dikonversi.
- FK nullable menjaga kompatibilitas; handler lama belum mengisi relasi ledger. Idempotensi/concurrency pembayaran, create order tanpa status, bootstrap owner aman dan risiko auth/privacy/XSS baseline tetap pekerjaan berikutnya. Aplikasi belum layak produksi.
