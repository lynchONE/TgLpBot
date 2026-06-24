package notify

import (
	"net/url"
	"strings"
	"testing"
)

func TestBarkEndpointWithConfigIncludesIntensityQuery(t *testing.T) {
	endpoint, ok := barkEndpointWithConfig("Title", "Body", BarkConfig{
		Server: "https://api.day.app",
		Key:    "abc123",
		Group:  "golden-dog",
		Sound:  "alarm",
		Call:   "1",
		Level:  "critical",
	})
	if !ok {
		t.Fatal("expected endpoint to be built")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	if got := parsed.Query().Get("sound"); got != "alarm" {
		t.Fatalf("expected sound=alarm, got %q", got)
	}
	if got := parsed.Query().Get("call"); got != "1" {
		t.Fatalf("expected call=1, got %q", got)
	}
	if got := parsed.Query().Get("level"); got != "critical" {
		t.Fatalf("expected level=critical, got %q", got)
	}
	if !strings.Contains(parsed.Path, "/abc123/") {
		t.Fatalf("expected key path segment, got %q", parsed.Path)
	}
}

func TestBarkEndpointWithConfigRemovesKeyFromServer(t *testing.T) {
	endpoint, ok := barkEndpointWithConfig("Title", "Body", BarkConfig{
		Server: "https://api.day.app/abc123",
		Key:    "abc123",
	})
	if !ok {
		t.Fatal("expected endpoint to be built")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	if got := parsed.Path; got != "/abc123/Title/Body" {
		t.Fatalf("expected normalized bark path, got %q", got)
	}
}

func TestBarkEndpointWithConfigKeepsServerPathPrefix(t *testing.T) {
	endpoint, ok := barkEndpointWithConfig("Title", "Body", BarkConfig{
		Server: "https://bark.example.com/push",
		Key:    "abc123",
	})
	if !ok {
		t.Fatal("expected endpoint to be built")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}

	if got := parsed.Path; got != "/push/abc123/Title/Body" {
		t.Fatalf("expected server path prefix to be preserved, got %q", got)
	}
}
