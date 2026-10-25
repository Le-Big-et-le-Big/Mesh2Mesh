// Command controlplane serves the Mesh2Mesh control-plane API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"Mesh2Mesh/internal/api"
	"Mesh2Mesh/internal/prober"
	"Mesh2Mesh/internal/relay"
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

	// The prober needs its own UDP socket to probe peer endpoints.
	probe, err := prober.Open(env("PROBE_ADDR", prober.DefaultAddr), log)
	if err != nil {
		return err
	}
	defer func() { _ = probe.Close() }()
	log.Info("endpoint prober listening", "addr", probe.LocalAddr().String())

	rel := relay.New(log)
	probe.SetRelay(rel)

	webToken := os.Getenv("WEB_API_TOKEN")
	if webToken == "" {
		log.Warn("WEB_API_TOKEN is not set: the web UI's per-user routes are off and anyone can list tenants")
	}

	proxies, err := parsePrefixes(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}

	addr := env("LISTEN_ADDR", defaultAddr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.TrustProxies(api.New(st, log, probe, rel, webToken).Routes(), proxies),
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

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parsePrefixes(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.Split(raw, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !strings.Contains(f, "/") {
			a, err := netip.ParseAddr(f)
			if err != nil {
				return nil, err
			}
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(f)
		if err != nil {
			return nil, err
		}
		out = append(out, p.Masked())
	}
	return out, nil
}
