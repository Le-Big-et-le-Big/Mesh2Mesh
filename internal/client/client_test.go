package client

import "testing"

func TestNewBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty falls back to default", "", DefaultBaseURL},
		{"blank falls back to default", "   ", DefaultBaseURL},
		{"host:port gains http", "192.168.122.62:8090", "http://192.168.122.62:8090"},
		{"hostname:port gains http", "controlplane:8090", "http://controlplane:8090"},
		{"bare hostname gains http", "localhost", "http://localhost"},
		{"http kept", "http://localhost:8090", "http://localhost:8090"},
		{"https kept", "https://cp.example.com", "https://cp.example.com"},
		{"trailing slash trimmed", "http://localhost:8090/", "http://localhost:8090"},
		{"trailing slash trimmed after scheme added", "localhost:8090/", "http://localhost:8090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.in).baseURL; got != tt.want {
				t.Errorf("New(%q).baseURL = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHasScheme(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"http://x", true},
		{"https://x", true},
		{"h+t-t.p://x", true},
		{"192.168.122.62:8090", false}, // scheme cannot start with a digit
		{"localhost:8090", false},      // "localhost:" is not followed by "//"
		{"localhost", false},
		{"", false},
		{"://x", false},
	}

	for _, tt := range tests {
		if got := hasScheme(tt.in); got != tt.want {
			t.Errorf("hasScheme(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
