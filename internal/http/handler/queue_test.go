package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ulaundry/internal/domain"
	"ulaundry/internal/repository"
	"ulaundry/internal/service"
)

func TestQueueSearchHTTP(t *testing.T) {
	h := New(Dependencies{Orders: fakeOrders{list: func(_ context.Context, actor string, f repository.OrderFilter) ([]service.OrderListItem, error) {
		if actor != "actor-1" || f.Search != "Antrean %_" || f.Status != domain.Ready || f.Page.Offset != 20 {
			t.Fatal("search/filter lost", f)
		}
		return nil, nil
	}}})
	w := protected(t, "GET", "/api/pesanan?search=Antrean%20%25_&status=ready&offset=20", "", h.GetSemuaPesanan)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, query := range []string{"search=x&search=y", "search=bad%0Aquery", "search=" + strings.Repeat("x", 101)} {
		w = protected(t, "GET", "/api/pesanan?"+query, "", h.GetSemuaPesanan)
		if w.Code != 400 {
			t.Fatal("bad search accepted", w.Code)
		}
	}
}

func TestQueueDetailAndThermalReceipt(t *testing.T) {
	o := sampleOrder(t)
	state := o.Snapshot()
	state.Items[0].ServiceName = "Cuci <img src=x onerror=alert(1)> " + strings.Repeat("W", 60)
	state.Notes = "Catatan <svg/onload=alert(1)>\n" + strings.TrimSpace(strings.Repeat("Catatan panjang aman. ", 30))
	longOrder, err := domain.RestoreOrder(state)
	if err != nil {
		t.Fatal(err)
	}
	h := New(Dependencies{Orders: fakeOrders{get: func(context.Context, string, string) (repository.OrderRecord, error) {
		return repository.OrderRecord{Order: o, Version: 1}, nil
	}}})
	w := protected(t, "GET", "/api/pesanan/order-1", "", h.GetPesananById)
	var detail struct {
		Paid, Balance int64
		Notes         string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || detail.Paid != 0 || detail.Balance != 17500 {
		t.Fatal("balance projection", w.Body.String())
	}
	for _, paper := range []string{"58", "80"} {
		w = protected(t, "GET", "/api/pesanan/order-1/struk?paper="+paper, "", h.CetakStruk)
		body := w.Body.String()
		for _, text := range []string{"paper-" + paper, "&lt;script&gt;private&lt;/script&gt;", "2500 gram", "Rp 17.500", "16:00 WITA", "/receipt.css", "/receipt.js"} {
			if !strings.Contains(body, text) {
				t.Fatal("receipt missing", text, body)
			}
		}
		if w.Code != 200 || strings.Contains(body, "onclick=") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("receipt safety")
		}
		// Optional review artifact, never enabled by the normal test command.
		if dir := os.Getenv("ULAUNDRY_RECEIPT_PREVIEW_DIR"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, "receipt-"+paper+".html"), w.Body.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
		longHandler := New(Dependencies{Orders: fakeOrders{get: func(context.Context, string, string) (repository.OrderRecord, error) {
			return repository.OrderRecord{Order: longOrder, Version: 1}, nil
		}}})
		longReceipt := protected(t, "GET", "/api/pesanan/order-1/struk?paper="+paper, "", longHandler.CetakStruk)
		if longReceipt.Code != 200 || strings.Contains(longReceipt.Body.String(), "<img") || strings.Contains(longReceipt.Body.String(), "<svg") || !strings.Contains(longReceipt.Body.String(), "&lt;img") || !strings.Contains(longReceipt.Body.String(), "&lt;svg") {
			t.Fatal("service/notes escaping failed", longReceipt.Body.String())
		}
		if dir := os.Getenv("ULAUNDRY_RECEIPT_PREVIEW_DIR"); dir != "" {
			if err := os.WriteFile(filepath.Join(dir, "receipt-long-"+paper+".html"), longReceipt.Body.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, query := range []string{"paper=57", "paper=58&paper=80", "paper=%3Cscript%3E", "unknown=1"} {
		w = protected(t, "GET", "/api/pesanan/order-1/struk?"+query, "", h.CetakStruk)
		if w.Code != 400 {
			t.Fatal("invalid paper accepted", w.Code)
		}
	}
}
