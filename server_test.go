package server_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

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
