package server_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nasermirzaei89/server"
)

func TestUnsupportedTLSModeError_Error(t *testing.T) {
	t.Parallel()

	err := server.UnsupportedTLSModeError{Mode: "invalid"}
	got := err.Error()
	want := `TLS mode "invalid" is not supported`

	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestRun_ReturnsUnsupportedTLSModeError(t *testing.T) {
	t.Parallel()

	srv := &server.Server{
		TLS: server.ServerTLS{
			Enabled: true,
			Mode:    "invalid-mode",
		},
	}

	err := srv.Run(context.Background(), http.NewServeMux())
	if err == nil {
		t.Error("expected error, got nil")
	}

	var modeErr *server.UnsupportedTLSModeError
	if !errors.As(err, &modeErr) {
		t.Errorf("expected UnsupportedTLSModeError, got %T", err)
	}

	if modeErr != nil && modeErr.Mode != "invalid-mode" {
		t.Errorf("expected mode %q, got %q", "invalid-mode", modeErr.Mode)
	}
}

func TestRun_SetsDefaultsBeforeStartFailure(t *testing.T) {
	t.Parallel()

	srv := &server.Server{
		Host: "bad host",
	}

	err := srv.Run(context.Background(), http.NewServeMux())
	if err == nil {
		t.Error("expected startup error, got nil")
	}

	if srv.Port != server.DefaultPort {
		t.Errorf("expected default port %q, got %q", server.DefaultPort, srv.Port)
	}

	if srv.TLS.Mode != server.DefaultTLSMode {
		t.Errorf("expected default TLS mode %q, got %q", server.DefaultTLSMode, srv.TLS.Mode)
	}

	if err != nil && !strings.Contains(err.Error(), "failed to start") {
		t.Errorf("expected startup failure message, got %v", err)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	addr := listener.Addr().String()

	err = listener.Close()
	if err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	return addr
}

func TestRunUnsecured_FinishesInFlightRequestOnShutdown(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	started := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)

		select {
		case <-time.After(200 * time.Millisecond):
			_, _ = w.Write([]byte("ok"))
		case <-r.Context().Done():
			http.Error(w, "request context canceled", http.StatusServiceUnavailable)
		}
	})

	var hookCalled atomic.Bool

	srv := &server.Server{
		OnShutdown: func() { hookCalled.Store(true) },
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)

	go func() {
		runErr <- srv.RunUnsecured(ctx, addr, handler)
	}()

	waitForListener(t, addr)

	type result struct {
		status int
		body   string
		err    error
	}

	resCh := make(chan result, 1)

	go func() {
		resp, err := http.Get("http://" + addr)
		if err != nil {
			resCh <- result{err: err}

			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		resCh <- result{status: resp.StatusCode, body: string(body), err: err}
	}()

	<-started
	cancel()

	res := <-resCh
	if res.err != nil {
		t.Fatalf("request failed: %v", res.err)
	}

	if res.status != http.StatusOK || res.body != "ok" {
		t.Errorf("expected 200 %q, got %d %q", "ok", res.status, res.body)
	}

	err := <-runErr
	if err != nil {
		t.Errorf("expected nil error on graceful shutdown, got %v", err)
	}

	if !hookCalled.Load() {
		t.Error("expected OnShutdown hook to be called")
	}
}

func waitForListener(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			_ = conn.Close()

			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("server did not start listening on %s", addr)
}
