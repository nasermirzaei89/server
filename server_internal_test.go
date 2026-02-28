package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDomainsToHTTPSAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		domains  []string
		expected string
	}{
		{
			name:     "single domain",
			domains:  []string{"example.com"},
			expected: "https://example.com",
		},
		{
			name:     "multiple domains",
			domains:  []string{"example.com", "www.example.com"},
			expected: "https://example.com, https://www.example.com",
		},
		{
			name:     "no domains",
			domains:  []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := domainsToHTTPSAddress(tt.domains)
			if tt.expected != result {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRunCancelable_ReturnsRunFuncError(t *testing.T) {
	t.Parallel()

	srv := &Server{Logger: newTestLogger()}
	expectedErr := errors.New("boom")

	err := srv.runCancelable(context.Background(), &http.Server{}, func() error {
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestRunCancelable_ShutsDownOnContextCancel(t *testing.T) {
	t.Parallel()

	srv := &Server{Logger: newTestLogger()}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	httpServer := &http.Server{Handler: http.NewServeMux()}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
		close(done)
	}()

	err = srv.runCancelable(ctx, httpServer, func() error {
		err := httpServer.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}

		return nil
	})
	if err != nil {
		t.Errorf("expected nil error on graceful shutdown, got %v", err)
	}

	<-done
}
