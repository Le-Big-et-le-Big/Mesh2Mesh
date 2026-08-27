// serves the Mesh2Mesh control-plane API: tenants
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"Mesh2Mesh/internal/api"
	"Mesh2Mesh/internal/prober"
	"Mesh2Mesh/internal/store"
)

const (
	defaultAddr = ":8090"
	defaultDSN  = "postgres://mesh:mesh@localhost:5432/mesh2mesh?sslmode=disable"

	shutdownTimeout = 10 * time.Second
	startupTimeout  = 10 * time.Second
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if err := run(log); err != nil {
		log.Error("controlplane stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dialCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	st, err := store.Open(dialCtx, env("DATABASE_URL", defaultDSN))
	if err != nil {
		return err
	}
	defer st.Close()

	// The prober needs its own UDP socket: it tests peer endpoints by sending
	// them a datagram and waiting for the echo.
	probe, err := prober.Open(env("PROBE_ADDR", prober.DefaultAddr), log)
	if err != nil {
		return err
	}
	defer probe.Close()
	log.Info("endpoint prober listening", "addr", probe.LocalAddr().String())

	addr := env("LISTEN_ADDR", defaultAddr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.New(st, log, probe).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("controlplane listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
		close(errc)
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()
	return srv.Shutdown(shutdownCtx)
}

// env reads an environment variable, falling back to def when it is unset.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
