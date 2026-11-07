# Laporan Hari 07 — Service dan Kepemilikan Workflow

## Tindakan — Hasil

- Pemisahan use case Customers/Catalog/Orders/Payments/Tracking/Reports/Auth/Health — workflow dan transaction boundary aktif di service, tidak digabung dalam satu service besar.
- Refactor handler dan composition root — handler hanya decode/delegasi/response/template; tidak menerima DB, menyusun SQL atau mengatur transaksi. Clock, ID/token generator, password verifier dan JWT issuer diinjeksi dari bootstrap.
- Kalkulasi dan kontrol workflow — harga dari katalog server, total/diskon domain, lock/version untuk status/configuration, cancel dengan alasan dan tanpa pembayaran, serta payment terpisah dari status operasional.
- Authorization dan audit — actor dimuat dari staf aktif pada setiap use case; intent/role eksplisit, aksi sensitif diaudit atomik. Kegagalan callback/commit tidak mengembalikan hasil sukses/JWT/token tracking; replay payment tidak menggandakan ledger/audit.
- Tracking/report — token acak 32 byte hanya disimpan sebagai SHA-256, projection publik tanpa PII/uang; cash-flow ledger memakai periode [from,to), default hari WITA, dengan scan error tetap menjadi kegagalan.
- Schema 000004 — version layanan, tracking token hash, allowed audit actions dan indeks report tersedia; migration 000001–000003 dan data legacy tetap.

## Verifikasi

- Targeted unit tests `internal/service` tanpa env DB — sembilan kelompok fake-repository lulus, mencakup 33 subkasus, total calculation, dependency failure, rollback intent, forbidden/stale transition, role, replay, privacy, report dan authentication.
- `go test ./internal/service -run '^TestPostgresPaymentUseCaseAtomicity$' -count=1 -timeout=90s` — satu alur PostgreSQL 16.14 terisolasi lulus: audit gagal membatalkan payment/ledger, retry dan replay menghasilkan satu posting/audit; configuration, tracking dan report pada alur memakai adapter nyata. Upgrade 3→4 mempertahankan history dan row legacy sintetis.
- Targeted tests domain/HTTP/middleware/app/platform/repository contract/migration catalog — lulus; handler fake membuktikan penolakan harga/total/role klien, filter/context, error redaction, receipt escaping dan private routes. JWT mengharuskan HS256/expiration/subject; full DB suites di-skip pada command unit.
- `gofmt`, `go build ./...`, `go vet` pada service/HTTP/app/platform/repository/migrate/server serta review boundary/import/diff — berhasil; tanpa dependency baru, full integration suite, UI/printer atau release/race checks.
- Cleanup database test — tidak ada database test tersisa, cluster test dihentikan dan chime `sounds/chime-1.mp3` selesai diputar dua kali; Git diff/status, SQL LF serta tautan dokumentasi diperiksa.

## Perubahan Penting

- `internal/service/` — workflow, permission intent, runtime dependencies, fake-unit dan satu integration test transaksi.
- `internal/http/handler/`, middleware/router, `internal/app/`, `cmd/server/` — wiring service aktif, endpoint workflow, actor dari JWT, DTO/response dan render struk; fixture SQL handler diganti fake use case.
- `internal/repository/`, `internal/repository/postgres/workflows.go`, domain audit, `internal/platform/workflow.go`, migration 000004 — kontrak pendukung, adapter/token/JWT/crypto dan schema additive.
- `docs/architecture.md`, `docs/api.md`, README/database/business-rules/backlog/audit — dependency direction, command, kontrak baru dan batas cutover diperbarui.

## Risiko atau Sisa Pekerjaan

- Wiring server beralih ke tabel domain — akun/data legacy belum diimpor, token lama tanpa subject tidak berlaku, dan bootstrap owner domain aman belum tersedia. Tidak melakukan import/cutover/deployment atau mengubah database pengguna; penggunaan nyata memerlukan backup/restore salinan, mapping dan rekonsiliasi satu writer.
- Kebijakan domain/role awal — cancellation/configuration/report owner/admin, pembulatan dan diskon nominal sudah dipakai kode baru; peninjauan pemilik diperlukan sebelum aktivasi operasional. Create order/customer belum idempotent dan hasil commit jaringan tidak pasti harus diperiksa sebelum retry.
- Hardening lanjutan — rate-limit/session/cookie/CSRF, user lifecycle, tracking expiry/rotation, frontend safe DOM/localStorage/POS, refund/shift/report lengkap serta TLS/role/restore/acceptance produksi belum ditutup. Tidak ada klaim siap produksi.
