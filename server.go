package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

const (
	HTTPServerTimeOut = 60 * time.Second
	ShutdownTimeout   = 5 * time.Second
)

const (
	TLSModeAutoCert = "autocert"
	TLSModeManual   = "manual"
)

const (
	DefaultPort    = "8080"
	DefaultTLSMode = TLSModeAutoCert
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// Server represents the HTTP server.
type Server struct {
	Port   string
	Host   string
	TLS    ServerTLS
	Logger *slog.Logger
}

type ServerTLS struct {
	Enabled  bool
	Mode     string
	AutoCert *ServerTLSAutoCert
	CertFile string
	KeyFile  string
}

type ServerTLSAutoCert struct {
	CacheDir string
	Domains  []string
	Email    string
}

type UnsupportedTLSModeError struct {
	Mode string
}

func (err UnsupportedTLSModeError) Error() string {
	return fmt.Sprintf("TLS mode %q is not supported", err.Mode)
}

func (server *Server) logger() *slog.Logger {
	if server.Logger != nil {
		return server.Logger
	}

	return discardLogger
}

// Run starts the HTTP server.
func (server *Server) Run(ctx context.Context, httpHandler http.Handler) error {
	if server.Port == "" {
		server.Port = DefaultPort
	}

	if server.TLS.Mode == "" {
		server.TLS.Mode = DefaultTLSMode
	}

	addr := server.Host + ":" + server.Port

	if server.TLS.Enabled {
		server.logger().DebugContext(ctx, "TLS is enabled")

		switch server.TLS.Mode {
		case TLSModeAutoCert:
			return server.RunAutoCert(ctx, addr, httpHandler)
		case TLSModeManual:
			return server.RunManualTLS(ctx, addr, httpHandler)
		default:
			return &UnsupportedTLSModeError{Mode: server.TLS.Mode}
		}
	}

	return server.RunUnsecured(ctx, addr, httpHandler)
}

func (server *Server) runAcmeChallengeServer(ctx context.Context, autocertManager *autocert.Manager) {
	httpHandler := autocertManager.HTTPHandler(nil) // serves /.well-known/acme-challenge/*

	const acmeChallengePort = "80"

	addr := ":" + acmeChallengePort

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           httpHandler,
		ReadTimeout:       HTTPServerTimeOut,
		ReadHeaderTimeout: HTTPServerTimeOut,
		WriteTimeout:      HTTPServerTimeOut,
		IdleTimeout:       HTTPServerTimeOut,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}

	err := server.runCancelable(ctx, httpServer, func() error {
		server.logger().InfoContext(ctx, "HTTP (ACME challenge) listening on "+addr)

		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed to start ACME challenge server: %w", err)
		}

		return nil
	})
	if err != nil {
		server.logger().ErrorContext(ctx, "ACME challenge server error", "error", err)
	}
}

// RunAutoCert starts the HTTP server with automatic TLS certificates using ACME.
func (server *Server) RunAutoCert(ctx context.Context, addr string, httpHandler http.Handler) error {
	autocertManager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(server.TLS.AutoCert.CacheDir), // where certs are stored on disk
		HostPolicy: autocert.HostWhitelist(server.TLS.AutoCert.Domains...),
		Email:      server.TLS.AutoCert.Email,
	}

	go server.runAcmeChallengeServer(ctx, autocertManager)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           httpHandler,
		ReadTimeout:       HTTPServerTimeOut,
		ReadHeaderTimeout: HTTPServerTimeOut,
		WriteTimeout:      HTTPServerTimeOut,
		IdleTimeout:       HTTPServerTimeOut,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		TLSConfig: &tls.Config{
			GetCertificate: autocertManager.GetCertificate,
			MinVersion:     tls.VersionTLS12,
		},
	}

	err := server.runCancelable(ctx, httpServer, func() error {
		address := domainsToHTTPSAddress(server.TLS.AutoCert.Domains)
		server.logger().InfoContext(ctx, "starting server", "address", address)

		err := httpServer.ListenAndServeTLS("", "")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed to start TLS server: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func domainsToHTTPSAddress(domains []string) string {
	prefixIter := func(yield func(string) bool) {
		for _, d := range domains {
			if !yield("https://" + d) {
				return
			}
		}
	}

	return strings.Join(slices.Collect(prefixIter), ", ")
}

// RunManualTLS starts the HTTP server with manually provided TLS certificates.
func (server *Server) RunManualTLS(ctx context.Context, addr string, httpHandler http.Handler) error {
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           httpHandler,
		ReadTimeout:       HTTPServerTimeOut,
		ReadHeaderTimeout: HTTPServerTimeOut,
		WriteTimeout:      HTTPServerTimeOut,
		IdleTimeout:       HTTPServerTimeOut,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	err := server.runCancelable(ctx, httpServer, func() error {
		server.logger().InfoContext(ctx, "starting server", "address", "https://"+addr)

		err := httpServer.ListenAndServeTLS(server.TLS.CertFile, server.TLS.KeyFile)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed to start TLS server: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// RunUnsecured starts the HTTP server without TLS.
func (server *Server) RunUnsecured(ctx context.Context, addr string, httpHandler http.Handler) error {
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           httpHandler,
		ReadTimeout:       HTTPServerTimeOut,
		ReadHeaderTimeout: HTTPServerTimeOut,
		WriteTimeout:      HTTPServerTimeOut,
		IdleTimeout:       HTTPServerTimeOut,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
	}

	err := server.runCancelable(ctx, httpServer, func() error {
		if strings.HasPrefix(addr, ":") {
			addr = "0.0.0.0" + addr
		}

		server.logger().InfoContext(ctx, "starting server", "address", "http://"+addr)

		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed to start server: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func (server *Server) runCancelable(ctx context.Context, httpServer *http.Server, runFunc func() error) error {
	errCh := make(chan error, 1)

	go func() {
		err := runFunc()
		if err != nil {
			errCh <- err
		}

		close(errCh)
	}()

	// Wait for interruption.
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		server.logger().InfoContext(shutdownCtx, "shutting down server...", "reason", ctx.Err())

		err := httpServer.Shutdown(shutdownCtx)
		if err != nil {
			return fmt.Errorf("error shutting down server: %w", err)
		}

		server.logger().InfoContext(shutdownCtx, "server shut down gracefully")

		return nil
	}
}
