package handler

import (
	"html/template"
	"strconv"
	"strings"
	"time"

	"ulaundry/internal/http/dto"
)

type receiptData struct {
	dto.OrderResponse
	Paper string
}

func receiptMoney(value int64) string {
	raw := strconv.FormatInt(value, 10)
	var groups []string
	for len(raw) > 3 {
		groups = append([]string{raw[len(raw)-3:]}, groups...)
		raw = raw[:len(raw)-3]
	}
	return "Rp " + strings.Join(append([]string{raw}, groups...), ".")
}

var receipt = template.Must(template.New("receipt").Funcs(template.FuncMap{
	"money": receiptMoney,
	"wita": func(at time.Time) string {
		return at.In(time.FixedZone("WITA", 8*60*60)).Format("02/01/2006 15:04") + " WITA"
	},
	"label": func(value string) string {
		if label, ok := map[string]string{"received": "Diterima", "processing": "Diproses", "ready": "Siap diambil", "collected": "Sudah diambil", "cancelled": "Dibatalkan", "unpaid": "Belum dibayar", "partially_paid": "Dibayar sebagian", "paid": "Lunas"}[value]; ok {
			return label
		}
		return value
	},
}).Parse(`<!doctype html><html lang="id"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex,nofollow"><title>Struk {{.ID}} — ULaundry</title><link rel="stylesheet" href="/receipt.css"><script src="/receipt.js" defer></script></head>
<body class="paper-{{.Paper}}"><nav class="print-controls" aria-label="Pengaturan cetak"><a href="?paper=58" {{if eq .Paper "58"}}aria-current="page"{{end}}>58 mm</a><a href="?paper=80" {{if eq .Paper "80"}}aria-current="page"{{end}}>80 mm</a><button id="print-receipt" type="button">Cetak struk</button><a href="/index.html">Antrean</a><p>Pilih kertas sesuai printer, skala 100%, matikan header/footer browser. Struk ini khusus staf.</p></nav>
<main><header><h1>ULAUNDRY</h1><p>Perumahan Viola</p><p>Struk pesanan</p></header><dl><dt>ID</dt><dd>{{.ID}}</dd><dt>Pelanggan</dt><dd>{{.Customer}}</dd><dt>Diterima</dt><dd>{{wita .CreatedAt}}</dd><dt>Estimasi</dt><dd>{{wita .EstimatedReadyAt}}</dd></dl>
<section aria-label="Rincian layanan">{{range .Items}}<article class="item"><strong>{{.Name}}</strong><p>{{if eq .Unit "kg"}}{{.Grams}} gram{{else}}{{.Pieces}} item{{end}} × {{money .UnitPrice}}/{{.Unit}}</p><p class="amount">{{money .LineTotal}}</p></article>{{end}}</section>
<dl class="totals"><dt>Subtotal</dt><dd>{{money .Subtotal}}</dd><dt>Diskon</dt><dd>{{money .Discount}}</dd><dt><strong>Total</strong></dt><dd><strong>{{money .Total}}</strong></dd><dt>Dibayar bersih</dt><dd>{{money .Paid}}</dd><dt>Sisa tagihan</dt><dd>{{money .Balance}}</dd></dl>
<p>Status: {{label .Status}}<br>Pembayaran: {{label .PaymentStatus}}</p>{{if .Notes}}<p class="notes">Catatan: {{.Notes}}</p>{{end}}{{if .CancellationReason}}<p>Alasan batal: {{.CancellationReason}}</p>{{end}}<footer><p>Terima kasih.<br>Simpan struk untuk pengambilan.</p></footer></main></body></html>`))
