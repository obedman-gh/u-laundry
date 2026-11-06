package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ulaundry/internal/config"
	"ulaundry/internal/http/handler"
)

type lifecycleDB struct {
	handler.Database // SQL is intentionally unavailable to lifecycle tests.
	closeCount       atomic.Int32
	closeErr         error
	pingCount        atomic.Int32
	pingErr          error
}

func (db *lifecycleDB) Ping() error {
	db.pingCount.Add(1)
	return db.pingErr
}

func (db *lifecycleDB) Close() error {
	db.closeCount.Add(1)
	return db.closeErr
}

func TestNewWiresDatabaseAndHTTP(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"healthy", nil, http.StatusOK},
		{"unavailable", errors.New("synthetic database failure"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &lifecycleDB{pingErr: tc.err}
			a := New(config.Config{AllowedOrigin: "https://fixture.invalid"}, db)
			rr := httptest.NewRecorder()
			a.server.Handler.ServeHTTP(rr, httptest.NewRequest("GET", "/api/health", nil))
			if rr.Code != tc.want || db.pingCount.Load() != 1 || rr.Header().Get("Access-Control-Allow-Origin") != "https://fixture.invalid" {
				t.Fatalf("application dependencies not wired: status=%d pings=%d", rr.Code, db.pingCount.Load())
			}
		})
	}
}

type observedListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (l *observedListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() { close(l.closed) })
	return err
}

func TestServeShutdown(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		name := "drains-before-closing-database"
		if timeout {
			name = "deadline-forces-http-close"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			observed := &observedListener{Listener: listener, closed: make(chan struct{})}
			grace := time.Second
			if timeout {
				grace = 20 * time.Millisecond
			}
			db := &lifecycleDB{}
			a := New(config.Config{HTTP: config.HTTP{ShutdownTimeout: grace}}, db)
			started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			releaseRequest := func() { releaseOnce.Do(func() { close(release) }) }
			a.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(finished)
				close(started)
				<-release
				w.WriteHeader(http.StatusNoContent)
			})
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(func() {
				cancel()
				releaseRequest()
				if err := observed.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
					t.Error(err)
				}
			})
			serveResult := make(chan error, 1)
			go func() { serveResult <- a.Serve(ctx, observed) }()
			transport := &http.Transport{Proxy: nil}
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			requestResult := make(chan error, 1)
			go func() {
				response, err := client.Get("http://" + listener.Addr().String())
				if err == nil {
					_, readErr := io.Copy(io.Discard, response.Body)
					err = errors.Join(readErr, response.Body.Close())
					if response.StatusCode != http.StatusNoContent {
						err = errors.Join(err, errors.New("unexpected response status"))
					}
				}
				requestResult <- err
			}()
			awaitSignal(t, started)
			cancel()
			awaitSignal(t, observed.closed)
			if !timeout {
				if count := db.closeCount.Load(); count != 0 {
					t.Fatal("database closed before in-flight request finished")
				}
				releaseRequest()
				awaitSignal(t, finished)
			}
			select {
			case err := <-serveResult:
				if timeout && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("expected shutdown deadline error, got %v", err)
				}
				if !timeout && err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("server did not stop")
			}
			if db.closeCount.Load() != 1 {
				t.Fatal("database must be closed exactly once")
			}
			releaseRequest()
			awaitSignal(t, finished)
			select {
			case err := <-requestResult:
				if !timeout && err != nil {
					t.Fatalf("graceful shutdown interrupted request: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("HTTP client did not finish")
			}
		})
	}
}

func awaitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for lifecycle signal")
	}
}

func TestRunClosesDatabaseOnListenFailure(t *testing.T) {
	closeErr := errors.New("synthetic close failure")
	db := &lifecycleDB{closeErr: closeErr}
	a := New(config.Config{HTTP: config.HTTP{Address: "127.0.0.1:invalid-port"}}, db)
	err := a.Run(context.Background())
	if !errors.Is(err, closeErr) || db.closeCount.Load() != 1 {
		t.Fatalf("listener failure lost cleanup error or DB close: %v", err)
	}
}

type failedListener struct{ err error }

func (l failedListener) Accept() (net.Conn, error) { return nil, l.err }
func (failedListener) Close() error                { return nil }
func (failedListener) Addr() net.Addr              { return &net.TCPAddr{} }

func TestServeReturnsAcceptAndCloseErrors(t *testing.T) {
	acceptErr := errors.New("synthetic listener failure")
	closeErr := errors.New("synthetic close failure")
	db := &lifecycleDB{closeErr: closeErr}
	a := New(config.Config{}, db)
	err := a.Serve(context.Background(), failedListener{err: acceptErr})
	if !errors.Is(err, acceptErr) || !errors.Is(err, closeErr) || db.closeCount.Load() != 1 {
		t.Fatalf("server/cleanup error not propagated: %v", err)
	}
}
