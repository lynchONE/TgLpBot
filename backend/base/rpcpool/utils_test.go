package rpcpool

import "testing"

func TestValidateURLForTransport(t *testing.T) {
	cases := []struct {
		name      string
		transport string
		url       string
		wantErr   bool
	}{
		{name: "http ok", transport: TransportHTTP, url: "https://example.com", wantErr: false},
		{name: "http bad scheme", transport: TransportHTTP, url: "wss://example.com", wantErr: true},
		{name: "ws ok", transport: TransportWS, url: "wss://example.com/ws", wantErr: false},
		{name: "ws bad scheme", transport: TransportWS, url: "https://example.com", wantErr: true},
		{name: "empty", transport: TransportHTTP, url: "", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateURLForTransport(c.url, c.transport)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
