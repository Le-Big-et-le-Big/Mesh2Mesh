package wire

import (
	"bytes"
	"errors"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	payload := []byte{0x45, 0x00, 0x00, 0x1c, 0xde, 0xad}

	typ, got, err := Decode(Encode(nil, TypeData, payload))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if typ != TypeData {
		t.Errorf("type = %d, want %d", typ, TypeData)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload = % x, want % x", got, payload)
	}
}

func TestEncodeReusesBuffer(t *testing.T) {
	// The forwarding path encodes into one buffer per loop, so a second call
	// must overwrite the first rather than append to it.
	buf := Encode(nil, TypeData, bytes.Repeat([]byte{1}, 100))
	buf = Encode(buf, TypeProbe, []byte{2, 2})

	if want := HeaderLen + 2; len(buf) != want {
		t.Fatalf("len = %d, want %d", len(buf), want)
	}
	typ, payload, err := Decode(buf)
	if err != nil || typ != TypeProbe || !bytes.Equal(payload, []byte{2, 2}) {
		t.Fatalf("Decode = (%d, % x, %v), want (%d, 02 02, nil)", typ, payload, err, TypeProbe)
	}
}

func TestDecodeRejectsBadFrames(t *testing.T) {
	tests := []struct {
		name  string
		frame []byte
		want  error
	}{
		{"empty", nil, ErrShortFrame},
		{"truncated header", []byte{Version, TypeData}, ErrShortFrame},
		{"future version", []byte{Version + 1, TypeData, 0, 0}, ErrVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := Decode(tt.frame); !errors.Is(err, tt.want) {
				t.Errorf("Decode err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewNonceIsUnique(t *testing.T) {
	a, err := NewNonce()
	if err != nil {
		t.Fatalf("NewNonce: %v", err)
	}
	b, err := NewNonce()
	if err != nil {
		t.Fatalf("NewNonce: %v", err)
	}
	if a == b {
		t.Error("two nonces came back equal")
	}
}
