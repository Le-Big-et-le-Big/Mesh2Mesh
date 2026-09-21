package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
)

// defaultCIDR is used when a tenant is created without one.
const defaultCIDR = "10.203.0.0/16"

// maxBodyBytes caps request bodies.
const maxBodyBytes = 64 << 10

// parseCIDR validates a tenant CIDR: a private IPv4 range with room for a
// couple of peers.
func parseCIDR(raw string) (netip.Prefix, error) {
	if strings.TrimSpace(raw) == "" {
		raw = defaultCIDR
	}

	prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("cidr is not a valid prefix: %v", err)
	}
	if !prefix.Addr().Is4() {
		return netip.Prefix{}, errors.New("cidr must be IPv4")
	}
	if !prefix.Addr().IsPrivate() {
		return netip.Prefix{}, errors.New("cidr must be a private range (10/8, 172.16/12 or 192.168/16)")
	}
	if prefix.Bits() > 30 {
		return netip.Prefix{}, errors.New("cidr must be /30 or larger")
	}
	return prefix.Masked(), nil
}

func decode(w http.ResponseWriter, r *http.Request, dst any, s *Server) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()

	switch err := dec.Decode(dst); {
	case errors.Is(err, io.EOF):
		return true // empty body: leave dst at its zero value
	case err != nil:
		s.fail(w, r, http.StatusBadRequest, "invalid JSON body: "+err.Error(), nil)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// fail logs the underlying cause and returns a JSON error to the client.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, status int, msg string, err error) {
	if err != nil {
		s.log.Error(msg, "err", err, "method", r.Method, "path", r.URL.Path, "status", status)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}
