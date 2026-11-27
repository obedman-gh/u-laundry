package telemetry

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRegistryConcurrentCountersAndBoundedLabels(t *testing.T) {
	m := NewRegistry()
	var group sync.WaitGroup
	for i := 0; i < 40; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			m.Observe("/api/pesanan", "POST", 201, 2*time.Millisecond, false)
			m.Business(OrderCreated)
		}()
	}
	group.Wait()
	s := m.Snapshot()
	if len(s.Requests) != 1 || s.Requests[0].Count != 40 || s.Requests[0].DurationMS != 80 || s.Requests[0].MaxDurationMS != 2 || s.Business.OrdersCreated != 40 {
		t.Fatalf("lost counters %+v", s)
	}
	m.Observe("/api/pesanan", "POST", 201, time.Millisecond, true)
	if m.Snapshot().Requests[0].Errors != 1 {
		t.Fatal("partial response failure not counted")
	}
	for i := 0; i < 800; i++ {
		m.Observe(fmt.Sprintf("controlled-fixture-%d", i), "nonstandard", 404, time.Millisecond, true)
	}
	s = m.Snapshot()
	if len(s.Requests) > 512 {
		t.Fatal("unbounded labels")
	}
	var total uint64
	for _, v := range s.Requests {
		total += v.Count
		if v.Method == "nonstandard" {
			t.Fatal("raw method retained")
		}
	}
	if total != 841 {
		t.Fatal("overflow lost events", total)
	}
	// A consumer cannot mutate the registry through its snapshot.
	s.Requests[0].Count = 0
	if m.Snapshot().Requests[0].Count == 0 {
		t.Fatal("snapshot aliases mutable registry")
	}
}
func TestJSONLoggerLevelsAndTransportRedaction(t *testing.T) {
	var out bytes.Buffer
	logger := NewLogger(&out, slog.LevelWarn)
	logger.Info("hidden")
	w := ServerErrorWriter{Logger: logger}
	private := "private-token password=private-db 192.0.2.50 /tracking/private"
	n, err := w.Write([]byte(private))
	if err != nil || n != len(private) {
		t.Fatal("writer contract")
	}
	var event map[string]any
	if err := json.Unmarshal(out.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event["level"] != "ERROR" || event["error_class"] != "transport" || strings.Contains(out.String(), "private") || strings.Contains(out.String(), "hidden") {
		t.Fatal("raw server diagnostic logged", out.String())
	}
}
func TestDBPoolProjection(t *testing.T) {
	p := DBPool(sql.DBStats{MaxOpenConnections: 10, OpenConnections: 7, InUse: 5, Idle: 2, WaitCount: 3, WaitDuration: 4 * time.Millisecond, MaxIdleClosed: 8, MaxIdleTimeClosed: 9, MaxLifetimeClosed: 1})
	if p.MaxOpen != 10 || p.Open != 7 || p.InUse != 5 || p.Idle != 2 || p.WaitCount != 3 || p.WaitDurationMS != 4 || p.MaxIdleClosed != 8 || p.MaxIdleTimeClosed != 9 || p.MaxLifetimeClosed != 1 {
		t.Fatalf("pool projection %+v", p)
	}
}
