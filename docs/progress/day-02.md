# Laporan Hari 02 — Fondasi Struktur dan Application Wiring

## Tindakan — Hasil

- Pemisahan entry point — server aktif di `cmd/server`, tiga utilitas di executable `cmd/legacy/` masing-masing; konflik empat fungsi main (H09) terselesaikan.
- Pemisahan bootstrap, router, config, database dan lifecycle — tersedia package internal dengan constructor/struct biasa, tanpa framework/dependency baru.
- Dependency injection handler/middleware — DB, JWT secret dan origin berasal bootstrap; server tidak memakai DB global atau membaca environment dari handler.
- Pemindahan handler secara struktural — SQL/aturan bisnis/response, sembilan URL API, static assets dan tema dipertahankan; service/repository belum diekstrak.
- Penggantian test lama — fixture SQL sintetis tanpa init/cwd/.env/DB nyata; jalur compatibility `config/database.go` dibatasi untuk utilitas dan dijadwalkan dihapus hari 03 sesudah import legacy dialihkan.

## Verifikasi

- `gofmt -w cmd config internal` dan pemeriksaan format — selesai; tidak ada file Go yang belum terformat pada area perubahan.
- `go build ./...` dengan Go 1.27.3, `GOTOOLCHAIN=local`, `GOPROXY=off`, cache di workspace — berhasil untuk seluruh package; tidak menjalankan utilitas atau menyimpan binary di Git.
- Targeted tests `internal/http`, `internal/http/handler`, `internal/http/middleware`, `internal/app` — lulus: sembilan route/batas auth/path ID, dua aset HTML asli/redirect, fixture DB independen/error query, JWT valid/expired/signature salah/secret hilang, wiring health dan lifecycle drain/deadline/listen-serve-close failure.
- Perilaku method lama — OPTIONS dan DELETE pada `/api/pesanan/ORD-123` menghasilkan 404 melalui static fallback; koreksi asumsi 405 pada temuan H11 dicatat tanpa mengubah router.
- Review sumber SQL, tautan dokumen, Git diff/status — 16 literal statement SQL pada enam sumber yang dipindahkan identik dengan baseline; frontend/model/migration/dependency tetap dan perubahan hari 01 dipertahankan.
- Chime `sounds/chime-1.mp3` — pemutar Windows MediaPlayer menyelesaikan playback dua kali.
- Tidak menjalankan full test suite, PostgreSQL integration, migration, utilitas setup, browser atau deployment. Koneksi SQL hanya diekstrak dengan DSN/Ping lama; tidak mengklaim perilaku PostgreSQL sudah dibuktikan.

## Perubahan Penting

- `cmd/server/main.go`, `internal/app/` — bootstrap/signal dan kepemilikan lifecycle HTTP/DB; shutdown normal menguras request sebelum close, kegagalan dilaporkan.
- `internal/config/`, `internal/platform/` — snapshot konfigurasi dan DB factory; validasi deployment/pool/SSL/context deadline tetap pekerjaan hari 03.
- `internal/http/` — router, handler, middleware dan test terisolasi; interface DB sementara tetap memakai tipe database/sql.
- `cmd/legacy/`, `config/database.go` — executable lama terpisah dan compatibility adapter khusus utilitas; tidak dipanggil server.
- `README.md`, `docs/target-architecture.md` — command `go run ./cmd/server`, struktur aktif, peta perpindahan dan rencana penghapusan; backlog/audit diperbarui untuk status H09 dan koreksi H11.

## Risiko atau Sisa Pekerjaan

- Risiko bisnis/security baseline masih terbuka: debit ganda, create/schema mismatch, filter status, XSS/localStorage, detail/struk publik, auth/role/CORS dan utilitas berkredensial tetap. Refactor wiring bukan persetujuan pemakaian produksi.
- Config masih default lama dan belum tervalidasi penuh; DB Ping/query belum context-aware dan SSL masih disable. Forced HTTP close setelah grace timeout tidak menjamin query lama berhenti; ditangani pada increment konfigurasi/repository.
- DB/schema aktual, full financial workflow, printer/mobile, Ubuntu/Cloudflare serta restore belum diverifikasi. Adapter global tersisa hanya di utilitas sampai hari 03; legacy DDL/bootstrap dihapus setelah pengganti hari 04/10 berfungsi.
