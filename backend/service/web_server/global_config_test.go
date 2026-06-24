package web_server

import (
	"encoding/json"
	"testing"
)

func TestBuildGlobalConfigUpdatesNormalizesOfficialBarkServer(t *testing.T) {
	raw := map[string]json.RawMessage{
		"bark_server": json.RawMessage(`"https://api.day.app/abc123/Title/Body"`),
	}

	updates := buildGlobalConfigUpdates(raw)

	if got := updates["bark_server"]; got != "https://api.day.app" {
		t.Fatalf("expected official bark server path to be stripped, got %#v", got)
	}
}

func TestBuildGlobalConfigUpdatesKeepsCustomBarkServerPath(t *testing.T) {
	raw := map[string]json.RawMessage{
		"bark_server": json.RawMessage(`"https://bark.example.com/push"`),
	}

	updates := buildGlobalConfigUpdates(raw)

	if got := updates["bark_server"]; got != "https://bark.example.com/push" {
		t.Fatalf("expected custom bark server path to be preserved, got %#v", got)
	}
}
