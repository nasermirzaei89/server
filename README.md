# server

Small Go library for running an `http.Handler` with:

- unsecured HTTP
- manual TLS certificate files
- automatic TLS via ACME (`autocert`)

## Install

```bash
go get github.com/nasermirzaei89/server
```

## Requirements

- Go `1.24+`
- For `autocert` mode:
  - valid public domains
  - incoming access to port `80` for ACME HTTP challenge
  - writable cache directory for certificates

## Quick Start (HTTP)

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/nasermirzaei89/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	srv := &server.Server{
		Port: "8080",
	}

	if err := srv.Run(ctx, handler); err != nil {
		log.Fatal(err)
	}
}
```

## Manual TLS Example

```go
srv := &server.Server{
	Host: "0.0.0.0",
	Port: "8443",
	TLS: server.ServerTLS{
		Enabled:  true,
		Mode:     server.TLSModeManual,
		CertFile: "/path/to/fullchain.pem",
		KeyFile:  "/path/to/privkey.pem",
	},
    Logger: slog.Default(),
}

if err := srv.Run(ctx, handler); err != nil {
	log.Fatal(err)
}
```

## AutoCert Example

```go
srv := &server.Server{
	Port: "443",
	TLS: server.ServerTLS{
		Enabled: true,
		Mode:    server.TLSModeAutoCert,
		AutoCert: &server.ServerTLSAutoCert{
			CacheDir: "./cert-cache",
			Domains:  []string{"example.com", "www.example.com"},
			Email:    "ops@example.com",
		},
	},
    Logger: slog.Default(),
}

if err := srv.Run(ctx, handler); err != nil {
	log.Fatal(err)
}
```

## Environment-based Config Example

```go
package main

import (
	"github.com/nasermirzaei89/env"
	"github.com/nasermirzaei89/server"
)

func newServer() *server.Server {
	srv := &server.Server{
		Port: env.GetString("PORT", server.DefaultPort),
		Host: env.GetString("HOST", ""),
		TLS: server.ServerTLS{
			Enabled: env.GetBool("TLS_ENABLED", false),
			Mode:    env.GetString("TLS_MODE", server.DefaultTLSMode),
			AutoCert: &server.ServerTLSAutoCert{
				CacheDir: env.GetString("TLS_AUTOCERT_CACHE_DIR", "./cert-cache"),
				Domains:  env.GetStringSlice("TLS_AUTOCERT_DOMAINS", []string{}),
				Email:    env.GetString("TLS_AUTOCERT_EMAIL", ""),
			},
			CertFile: env.GetString("TLS_CERT_FILE", ""),
			KeyFile:  env.GetString("TLS_KEY_FILE", ""),
		},
        Logger: slog.Default(),
	}

	return srv
}
```

## Defaults

- `Port`: `8080` when empty
- `TLS.Mode`: `autocert` when empty
- HTTP server read/write/idle timeout: `60s`
- graceful shutdown timeout: `5s`

## API Summary

- `type Server`
- `func (s *Server) Run(ctx context.Context, httpHandler http.Handler) error`
- `const TLSModeAutoCert = "autocert"`
- `const TLSModeManual = "manual"`

## Notes

- `Run` blocks until:
  - server startup fails, or
  - context is canceled and graceful shutdown completes.
- In `autocert` mode, an additional HTTP server is started on port `80` for ACME challenge handling.
