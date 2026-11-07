# Laporan Hari 08 — HTTP API, Error Contract dan Request Safety

## Tindakan — Hasil

- Pemeriksaan repository — hari 01–07 tercatat selesai; partial DTO/boundary/response hari 08 sudah ada, tetapi belum ter-wire penuh atau terdokumentasi. Banyak perubahan lokal dipertahankan; salinan HTTP/dokumen dan file yang disentuh tersedia di backups/before-day08-20261108-030256 pada root workspace.
- Penyelesaian HTTP boundary — handler memakai DTO terpisah/validasi boundary, error JSON berkode/message aman/request_id dan response helper; 64 KiB, UTF-8, content type/encoding, field unknown/duplikat/nested, pagination/query/path serta satu Idempotency-Key diverifikasi sebelum use case.
- Router/middleware — namespace API terpisah dari FileServer; 404 JSON, 405 dengan Allow, HEAD tanpa body, ID server pada error auth/CORS/router dan recovery panic tanpa detail internal. Preflight tetap sebelum auth; akses resource/role tetap di use case.
- Kompatibilitas — alias filter lama tetap dengan Deprecation/Link, status payment/replay 201/200, token tracking canonical dan DTO projection publik minimal. Pesan login membaca error.message dengan textContent/fallback; tema tidak berubah.

## Verifikasi

- go test ./internal/http/... ./internal/app -count=1 -timeout=45s — lulus pada Go 1.27.3 Windows; fake service/lifecycle, tanpa .env/DB. Kasus mencakup success, invalid/type/unknown/duplicate JSON, body tepat/melewati 64 KiB termasuk stream, query/path, typed error/redaction, auth/CORS, 404/405/HEAD, idempotency header/replay, tracking privacy, receipt escaping dan recovery.
- node --test scripts/frontend-contract.test.cjs — 4/4 lulus: nested error sebagai teks, fallback objek tak dikenal, login sukses dan kegagalan jaringan; DOM/fetch/storage sintetis, bukan browser.
- go build ./... dan go vet ./internal/http/... ./internal/app — lulus, dependency dan migration tidak berubah. Gofmt area HTTP/app test, review diff serta whitespace/dokumentasi diperiksa.
- Dua kegagalan awal diselesaikan — fixture tracking lama memakai token noncanonical dan assertion health masih mengharapkan 500; fixture valid sintetis dan status layanan 503 kini sesuai kontrak. Tidak memakai akun/token nyata.

## Perubahan Penting

- internal/http/router.go, middleware/, handler/boundary.go, dto/ dan response/ — integrasi fondasi partial hari 08 dan hardening request/response/routing; handler tetap bebas SQL/transaksi.
- internal/http/*_test.go, handler/boundary_test.go, middleware/requests_test.go, response/response_test.go, internal/app/app_test.go — bukti kontrak dan regresi terarah.
- frontend/login.html dan scripts/frontend-contract.test.cjs — kompatibilitas error terstruktur dan empat test tanpa dependency frontend baru.
- README.md, docs/api.md, architecture/backlog/audit dan laporan ini — status 01–08 selesai pada scope masing-masing; 09–30 tetap pekerjaan berikutnya.

## Risiko atau Sisa Pekerjaan

- Berikutnya hari 09: rate limit login, issuer/audience/issued-at, cookie/CSRF, logout/revocation dan dokumentasi security; lalu hari 10 bootstrap owner/manajemen staf. Tidak ada klaim siap produksi.
- Import/cutover akun/data legacy, idempotensi create order/customer, refund/shift, UI POS/safe DOM daftar, PWA, backup/restore/deployment dan acceptance 09–30 belum selesai. Tidak mengubah .env, database, akun/sesi atau layanan pengguna; tidak commit/push.
- PostgreSQL integration, browser/mobile/visual, printer, race dan release checks tidak dijalankan karena perubahan hanya transport/pesan login; bukti DB hari sebelumnya tidak diklaim diuji ulang. Request ID belum terhubung ke request logging terstruktur. Error jaringan setelah commit tidak menjamin mutasi belum terjadi.
